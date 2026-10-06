import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve } from 'path'

// dist 产物目录：Go 侧 //go:embed all:dist 嵌入的就是这里
const distDir = resolve(__dirname, '../internal/webapi/dist')

/**
 * 构建结束后补回 dist/.gitkeep。
 *
 * 原因：emptyOutDir 会把整个 dist 清空，连占位文件一起删掉；
 * 而仓库里 dist 只跟踪这个占位（//go:embed all:dist 要求目录非空），
 * 一旦它被删又被提交，干净检出就会重新回到 `pattern all:dist: no matching files found` 的破状态。
 * 靠人记住"构建后别提交 .gitkeep 的删除"不可靠，所以交给构建流程本身保证。
 */
function keepDistPlaceholder(outDir: string): Plugin {
  return {
    name: 'wukong:keep-dist-placeholder',
    // closeBundle 在所有产物写盘完成后执行，正好补被清掉的占位
    async closeBundle() {
      await mkdir(outDir, { recursive: true })
      await writeFile(
        resolve(outDir, '.gitkeep'),
        '# 该文件仅用于让 //go:embed all:dist 在干净检出时仍能编译通过。\n' +
          '# 真正的前端产物由 `make build-frontend`（或 CI 的 node 阶段）生成，不入库。\n',
        'utf8'
      )
    },
  }
}

export default defineConfig({
  plugins: [vue(), keepDistPlaceholder(distDir)],
  base: '/',
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 5173,
    host: '0.0.0.0',
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:64443',
        changeOrigin: true,
      },
    },
  },
  build: {
    // 保留 emptyOutDir：否则带 hash 的旧资源会不断累积，嵌入二进制后白白增大体积
    outDir: '../internal/webapi/dist',
    emptyOutDir: true,
    sourcemap: false,
  },
})