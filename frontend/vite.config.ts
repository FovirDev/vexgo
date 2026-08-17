import path from "path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import { inspectAttr } from "kimi-plugin-inspect-react";

// https://vite.dev/config/
export default defineConfig({
  base: "/",
  plugins: [inspectAttr(), react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "../backend/internal/public/dist",
    // outDir 位于项目根之外，vite 默认不会清空旧产物。
    // 不清空会导致 dist 里堆积多次构建的文件，
    // 后端 buildAssetManifest 按字母序取同名资源时会选中旧 bundle，
    // 造成 SSR 页面加载到过期前端代码。
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          // 将 React 相关库单独打包
          "react-vendor": ["react", "react-dom", "react-router-dom"],
          // 将 UI 组件库单独打包
          "ui-vendor": [
            "@radix-ui/react-slot",
            "class-variance-authority",
            "clsx",
            "tailwind-merge",
            "lucide-react",
          ],
          // 将状态管理和工具库单独打包
          "utils-vendor": ["axios", "date-fns"],
        },
      },
    },
    // 调整警告阈值（可选）
    chunkSizeWarningLimit: 600,
    // 生成 manifest.json，供后端 SSR 精确引用正确的入口与 vendor 资源。
    // 不能依赖后端按字母序扫描 assets 目录：一次构建会产生多个
    // index-<hash>.js（懒加载分块），按名称映射会选到错误的文件。
    manifest: true,
  },
});
