// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docs: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      link: {
        type: 'generated-index',
        description: 'Learn how to install and configure Stratos.',
      },
      items: [
        'getting-started/installation',
        'getting-started/configuration',
        'getting-started/first-nodepool',
      ],
    },
    {
      type: 'category',
      label: 'Concepts',
      link: {
        type: 'generated-index',
        description: 'Understand Stratos architecture and core concepts.',
      },
      items: [
        'concepts/architecture',
        'concepts/node-lifecycle',
        'concepts/cloud-providers',
      ],
    },
    {
      type: 'category',
      label: 'Guides',
      link: {
        type: 'generated-index',
        description: 'Step-by-step guides for common tasks.',
      },
      items: [
        'guides/aws-setup',
        'guides/scaling-policies',
        'guides/monitoring',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      link: {
        type: 'generated-index',
        description: 'API reference and configuration options.',
      },
      items: [
        {
          type: 'category',
          label: 'API Reference',
          link: {
            type: 'generated-index',
            description: 'Custom Resource Definition API reference.',
          },
          items: [
            'reference/api/nodepool',
          ],
        },
        'reference/cli',
        'reference/labels-annotations',
      ],
    },
    {
      type: 'category',
      label: 'Development',
      link: {
        type: 'generated-index',
        description: 'Contributing to Stratos development.',
      },
      items: [
        'development/contributing',
        'development/local-development',
        'development/testing',
      ],
    },
  ],
};

module.exports = sidebars;
