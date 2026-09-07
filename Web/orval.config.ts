import { defineConfig } from 'orval'

export default defineConfig({
  releaseHub: {
    input: {
      target: '../API/openapi.yaml',
    },
    output: {
      mode: 'single',
      target: './src/generated/api.ts',
      schemas: './src/generated/model',
      client: 'react-query',
      httpClient: 'fetch',
      clean: true,
    },
  },
})
