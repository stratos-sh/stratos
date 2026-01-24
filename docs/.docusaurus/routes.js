import React from 'react';
import ComponentCreator from '@docusaurus/ComponentCreator';

export default [
  {
    path: '/__docusaurus/debug',
    component: ComponentCreator('/__docusaurus/debug', '5ff'),
    exact: true
  },
  {
    path: '/__docusaurus/debug/config',
    component: ComponentCreator('/__docusaurus/debug/config', '5ba'),
    exact: true
  },
  {
    path: '/__docusaurus/debug/content',
    component: ComponentCreator('/__docusaurus/debug/content', 'a2b'),
    exact: true
  },
  {
    path: '/__docusaurus/debug/globalData',
    component: ComponentCreator('/__docusaurus/debug/globalData', 'c3c'),
    exact: true
  },
  {
    path: '/__docusaurus/debug/metadata',
    component: ComponentCreator('/__docusaurus/debug/metadata', '156'),
    exact: true
  },
  {
    path: '/__docusaurus/debug/registry',
    component: ComponentCreator('/__docusaurus/debug/registry', '88c'),
    exact: true
  },
  {
    path: '/__docusaurus/debug/routes',
    component: ComponentCreator('/__docusaurus/debug/routes', '000'),
    exact: true
  },
  {
    path: '/',
    component: ComponentCreator('/', '2b0'),
    routes: [
      {
        path: '/',
        component: ComponentCreator('/', 'd6b'),
        routes: [
          {
            path: '/',
            component: ComponentCreator('/', 'e0d'),
            routes: [
              {
                path: '/category/api-reference',
                component: ComponentCreator('/category/api-reference', '465'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/category/concepts',
                component: ComponentCreator('/category/concepts', '3cc'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/category/development',
                component: ComponentCreator('/category/development', '489'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/category/getting-started',
                component: ComponentCreator('/category/getting-started', '9fd'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/category/guides',
                component: ComponentCreator('/category/guides', 'ade'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/category/reference',
                component: ComponentCreator('/category/reference', 'c8e'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/concepts/architecture',
                component: ComponentCreator('/concepts/architecture', '683'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/concepts/cloud-providers',
                component: ComponentCreator('/concepts/cloud-providers', '544'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/concepts/node-lifecycle',
                component: ComponentCreator('/concepts/node-lifecycle', '797'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/development/contributing',
                component: ComponentCreator('/development/contributing', '121'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/development/local-development',
                component: ComponentCreator('/development/local-development', '9c8'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/development/testing',
                component: ComponentCreator('/development/testing', '441'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/getting-started/configuration',
                component: ComponentCreator('/getting-started/configuration', '7fb'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/getting-started/first-nodepool',
                component: ComponentCreator('/getting-started/first-nodepool', 'cd3'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/getting-started/installation',
                component: ComponentCreator('/getting-started/installation', '69f'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/guides/aws-setup',
                component: ComponentCreator('/guides/aws-setup', '6b7'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/guides/monitoring',
                component: ComponentCreator('/guides/monitoring', '941'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/guides/scaling-policies',
                component: ComponentCreator('/guides/scaling-policies', 'dd8'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/reference/api/nodepool',
                component: ComponentCreator('/reference/api/nodepool', '84c'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/reference/cli',
                component: ComponentCreator('/reference/cli', '207'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/reference/labels-annotations',
                component: ComponentCreator('/reference/labels-annotations', '46a'),
                exact: true,
                sidebar: "docs"
              },
              {
                path: '/',
                component: ComponentCreator('/', 'daa'),
                exact: true,
                sidebar: "docs"
              }
            ]
          }
        ]
      }
    ]
  },
  {
    path: '*',
    component: ComponentCreator('*'),
  },
];
