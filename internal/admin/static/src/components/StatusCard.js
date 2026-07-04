import { h } from "../lib/react.js";

export function StatusCard({ label, value, detail, tone = "neutral" }) {
  return h(
    "article",
    { className: `status-card ${tone}` },
    h("span", { className: "status-label" }, label),
    h("strong", null, value),
    h("small", null, detail),
  );
}
