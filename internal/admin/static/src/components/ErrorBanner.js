import { h } from "../lib/react.js";

export function ErrorBanner({ title, message }) {
  return h(
    "div",
    { className: "error-banner", role: "alert" },
    h("strong", null, title),
    h("span", null, message),
  );
}
