import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Aglet',
  description: 'A protocol for self-describing, agent-native software',

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
          text: 'Guide',
          items: [
            { text: 'What is Aglet?', link: '/guide/what-is-aglet' },
            { text: 'Getting Started', link: '/guide/getting-started' },
            { text: 'How It Works', link: '/guide/how-it-works' },
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
            { text: 'Components', link: '/spec/components' },
            { text: 'Domains', link: '/spec/domains' },
            { text: 'Intent', link: '/spec/intent' },
            { text: 'Runtime Architecture', link: '/spec/runtime' },
            { text: 'Adaptive Memory Layer', link: '/spec/aml' },
            { text: 'Guardrails', link: '/spec/guardrails' },
            { text: 'Future Features', link: '/spec/future' },
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
