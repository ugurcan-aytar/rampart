// Side-effect CSS imports from third-party packages (reactflow ships
// `reactflow/dist/style.css`). TypeScript needs a module declaration to
// permit `import 'pkg/style.css'` without a real .d.ts in the package.
declare module '*.css';
