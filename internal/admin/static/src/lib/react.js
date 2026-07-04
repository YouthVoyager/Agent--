const globalReact = window.React;
const globalReactDOM = window.ReactDOM;

if (!globalReact || !globalReactDOM) {
  throw new Error("React 静态资源加载失败");
}

export const React = globalReact;
export const Fragment = globalReact.Fragment;
export const createRoot = globalReactDOM.createRoot;
export const h = globalReact.createElement;
export const useCallback = globalReact.useCallback;
export const useEffect = globalReact.useEffect;
export const useState = globalReact.useState;
