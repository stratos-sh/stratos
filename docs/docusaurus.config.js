// @ts-check
// Docusaurus configuration for Stratos documentation

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Stratos',
  tagline: 'Eliminate Kubernetes node cold-start delays',
  favicon: 'img/favicon.ico',

  // Set the production url of your site here
  url: 'https://stratos.sh',
  // Set the /<baseUrl>/ pathname under which your site is served
  baseUrl: '/',

  // GitHub pages deployment config
  organizationName: 'stratos-sh',
  projectName: 'stratos',

  onBrokenLinks: 'throw',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          path: '.',
          sidebarPath: './sidebars.js',
          routeBasePath: '/',
          editUrl: 'https://github.com/stratos-sh/stratos/tree/main/docs/',
          exclude: ['**/node_modules/**', 'src/**', 'static/**', 'package*.json', 'sidebars.js', 'docusaurus.config.js'],
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      // Social card image
      image: 'img/stratos-social-card.png',
      navbar: {
        title: 'Stratos',
        logo: {
          alt: 'Stratos Logo',
          src: 'img/logo.png',
        },
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docs',
            position: 'left',
            label: 'Documentation',
          },
          {
            href: 'https://github.com/stratos-sh/stratos',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Documentation',
            items: [
              {
                label: 'Getting Started',
                to: '/getting-started/installation',
              },
              {
                label: 'Concepts',
                to: '/concepts/architecture',
              },
              {
                label: 'API Reference',
                to: '/reference/api/nodepool',
              },
            ],
          },
          {
            title: 'Community',
            items: [
              {
                label: 'GitHub',
                href: 'https://github.com/stratos-sh/stratos',
              },
              {
                label: 'Issues',
                href: 'https://github.com/stratos-sh/stratos/issues',
              },
            ],
          },
        ],
        copyright: `Copyright ${new Date().getFullYear()} Stratos Authors. Built with Docusaurus.`,
      },
      prism: {
        additionalLanguages: ['bash', 'yaml', 'json', 'go'],
      },
    }),
};

module.exports = config;
