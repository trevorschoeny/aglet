import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Aglet',
  description: 'A protocol for observable, agent-native software',

  // Deploy to GitHub Pages at /aglet/
  base: '/aglet/',

  // Clean URLs (no .html suffix)
  cleanUrls: true,

  themeConfig: {
    nav: [
      { text: 'Guide', link: '/guide/what-is-aglet' },
      { text: 'Specification', link: '/spec/blocks' },
      { text: 'CLI', link: '/cli/' },
      {
        text: 'Links',
        items: [
          { text: 'GitHub', link: 'https://github.com/trevorschoeny/aglet' },
          { text: 'Install', link: '/guide/getting-started' },
        ]
      }
    ],

    sidebar: {
      '/guide/': [
        {
          text: 'Introduction',
          items: [
            { text: 'What is Aglet?', link: '/guide/what-is-aglet' },
            { text: 'Getting Started', link: '/guide/getting-started' },
            { text: 'Core Concepts', link: '/guide/core-concepts' },
            { text: 'Agent Setup', link: '/guide/agent-setup' },
          ]
        }
      ],
      '/spec/': [
        {
          text: 'Specification',
          items: [
            { text: 'Blocks', link: '/spec/blocks' },
            { text: 'Surfaces', link: '/spec/surfaces' },
            { text: 'Domains', link: '/spec/domains' },
            { text: 'Intent', link: '/spec/intent' },
            { text: 'Guardrails', link: '/spec/guardrails' },
            { text: 'Adaptive Memory Layer', link: '/spec/aml' },
          ]
        }
      ],
      '/cli/': [
        {
          text: 'CLI Reference',
          items: [
            { text: 'Commands', link: '/cli/' },
          ]
        }
      ],
      '/patterns/': [
        {
          text: 'Patterns',
          items: [
            { text: 'Overview', link: '/patterns/' },
          ]
        }
      ],
      '/examples/': [
        {
          text: 'Examples',
          items: [
            { text: 'Overview', link: '/examples/' },
          ]
        }
      ],
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/trevorschoeny/aglet' }
    ],

    search: {
      provider: 'local'
    },

    footer: {
      message: 'Released under the MIT License.',
    }
  }
})
