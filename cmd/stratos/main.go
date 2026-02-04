/*
Copyright 2026 Stratos Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"os"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	ec2svc "github.com/aws/aws-sdk-go-v2/service/ec2"
	iamsvc "github.com/aws/aws-sdk-go-v2/service/iam"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider/aws"
	"github.com/stratos-sh/stratos/internal/config"
	"github.com/stratos-sh/stratos/internal/controller"
	// Import metrics package to register metrics
	_ "github.com/stratos-sh/stratos/internal/metrics"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	// Build info (set by ldflags)
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// cliConfig holds all CLI flag values parsed from flags and environment.
type cliConfig struct {
	metricsAddr             string
	probeAddr               string
	enableLeaderElection    bool
	syncPeriod              time.Duration
	clusterName             string
	clusterEndpoint         string
	clusterCA               string
	clusterCIDR             string
	cloudProvider           string
	gracefulShutdownTimeout time.Duration
	awsRateLimitQPS         float64
	awsRateLimitBurst       float64
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(stratosv1alpha1.AddToScheme(scheme))
}

// parseFlags loads configuration from environment and CLI flags, returning the merged config.
func parseFlags() cliConfig {
	cfg := config.LoadFromEnv()

	var cli cliConfig
	flag.StringVar(&cli.metricsAddr, "metrics-bind-address", cfg.MetricsBindAddress, "The address the metric endpoint binds to.")
	flag.StringVar(&cli.probeAddr, "health-probe-bind-address", cfg.HealthProbeBindAddress, "The address the probe endpoint binds to.")
	flag.BoolVar(&cli.enableLeaderElection, "leader-elect", cfg.LeaderElect,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&cli.syncPeriod, "sync-period", cfg.SyncPeriod, "The minimum interval at which watched resources are reconciled.")
	flag.StringVar(&cli.clusterName, "cluster-name", cfg.ClusterName, "The name of the Kubernetes cluster (required for cloud provider tagging).")
	flag.StringVar(&cli.clusterEndpoint, "cluster-endpoint", cfg.ClusterEndpoint, "The Kubernetes API server endpoint (e.g., https://...).")
	flag.StringVar(&cli.clusterCA, "cluster-ca", cfg.ClusterCA, "Base64-encoded CA certificate for the Kubernetes API server.")
	flag.StringVar(&cli.clusterCIDR, "cluster-cidr", cfg.ClusterCIDR, "The cluster service CIDR (e.g., 172.20.0.0/16).")
	flag.StringVar(&cli.cloudProvider, "cloud-provider", cfg.CloudProvider, "The cloud provider to use (aws, fake).")
	flag.DurationVar(&cli.gracefulShutdownTimeout, "graceful-shutdown-timeout", cfg.GracefulShutdownTimeout, "The timeout for graceful shutdown of the manager.")
	flag.Float64Var(&cli.awsRateLimitQPS, "aws-rate-limit-qps", cfg.AWSRateLimitQPS, "Multiplier for AWS API rate limit refill rate (1.0 = AWS defaults).")
	flag.Float64Var(&cli.awsRateLimitBurst, "aws-rate-limit-burst", cfg.AWSRateLimitBurst, "Multiplier for AWS API rate limit bucket size (1.0 = AWS defaults).")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if cli.clusterName == "" {
		setupLog.Info("Warning: cluster-name not set, using 'default'")
		cli.clusterName = "default"
	}

	return cli
}

// buildClusterConfig creates and validates the cluster config for AWS provider.
// Returns nil for non-AWS providers.
func buildClusterConfig(cli cliConfig) *config.ClusterConfig {
	if cli.cloudProvider != "aws" {
		return nil
	}

	k8sVersion, err := config.DetectKubernetesVersion(context.Background())
	if err != nil {
		setupLog.Error(err, "failed to detect Kubernetes version (AMI auto-discovery will be unavailable)")
		k8sVersion = ""
	} else {
		setupLog.Info("detected Kubernetes version", "version", k8sVersion)
	}

	clusterConfig := &config.ClusterConfig{
		Name:                 cli.clusterName,
		APIServerEndpoint:    cli.clusterEndpoint,
		CertificateAuthority: cli.clusterCA,
		CIDR:                 cli.clusterCIDR,
		KubernetesVersion:    k8sVersion,
	}
	if err := clusterConfig.Validate(); err != nil {
		setupLog.Error(err, "invalid cluster configuration")
		os.Exit(1)
	}
	setupLog.Info("cluster config validated",
		"name", cli.clusterName,
		"endpoint", cli.clusterEndpoint,
		"cidr", cli.clusterCIDR,
		"k8sVersion", k8sVersion,
	)

	return clusterConfig
}

// registerAWSNodeClassReconciler sets up the AWSNodeClass reconciler when using the AWS provider.
func registerAWSNodeClassReconciler(mgr ctrl.Manager, cli cliConfig, clusterConfig *config.ClusterConfig, rateLimitCfg *aws.RateLimitConfig) {
	if cli.cloudProvider != "aws" {
		return
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		setupLog.Error(err, "unable to load AWS config for AWSNodeClass reconciler")
		os.Exit(1)
	}
	ec2Client := ec2svc.NewFromConfig(awsCfg)
	iamClient := iamsvc.NewFromConfig(awsCfg)
	resolver := aws.NewAWSResolver(ec2Client, iamClient, aws.NewRateLimiter(rateLimitCfg))

	if err = (&aws.AWSNodeClassReconciler{
		Client:            mgr.GetClient(),
		Resolver:          resolver,
		ClusterName:       cli.clusterName,
		KubernetesVersion: clusterConfig.KubernetesVersion,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AWSNodeClass")
		os.Exit(1)
	}
}

func main() {
	cli := parseFlags()

	setupLog.Info("starting stratos controller",
		"version", version,
		"commit", commit,
		"buildDate", buildDate,
	)

	clusterConfig := buildClusterConfig(cli)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: cli.metricsAddr,
		},
		HealthProbeBindAddress:  cli.probeAddr,
		LeaderElection:          cli.enableLeaderElection,
		LeaderElectionID:        "stratos.sh",
		GracefulShutdownTimeout: &cli.gracefulShutdownTimeout,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	rateLimitCfg := &aws.RateLimitConfig{
		QPS:   cli.awsRateLimitQPS,
		Burst: cli.awsRateLimitBurst,
	}

	if err = controller.Setup(mgr, controller.SetupOptions{
		ClusterName:      cli.clusterName,
		CloudProvider:    cli.cloudProvider,
		ClusterConfig:    clusterConfig,
		CapacityProvider: aws.GetInstanceCapacity,
		CNIPodSelector:   map[string]string{"k8s-app": "aws-node"},
		RateLimitConfig:  rateLimitCfg,
	}); err != nil {
		setupLog.Error(err, "unable to create controllers")
		os.Exit(1)
	}

	registerAWSNodeClassReconciler(mgr, cli, clusterConfig, rateLimitCfg)

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	setupLog.Info("manager stopped gracefully")
}
