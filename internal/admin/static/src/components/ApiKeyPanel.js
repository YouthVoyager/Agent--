import { h, useEffect, useState } from "../lib/react.js";

export function ApiKeyPanel({ apiKey, onApiKeyChange }) {
  const [draftApiKey, setDraftApiKey] = useState(apiKey);
  const hasApiKey = apiKey.trim() !== "";

  useEffect(() => {
    setDraftApiKey(apiKey);
  }, [apiKey]);

  return h(
    "section",
    { className: "panel" },
    h(
      "div",
      { className: "panel-heading" },
      h("h2", null, "访问凭据"),
      h("span", { className: hasApiKey ? "pill success" : "pill neutral" }, hasApiKey ? "已配置" : "未配置"),
    ),
    h(
      "form",
      {
        className: "api-key-form",
        onSubmit: (event) => {
          event.preventDefault();
          onApiKeyChange(draftApiKey);
        },
      },
      h(
        "label",
        { className: "field" },
        h("span", null, "API Key"),
        h("input", {
          type: "password",
          value: draftApiKey,
          autoComplete: "off",
          placeholder: "Bearer 或原始 key",
          onChange: (event) => setDraftApiKey(event.target.value),
        }),
      ),
      h(
        "div",
        { className: "button-row" },
        h("button", { className: "primary-button", type: "submit" }, "保存"),
        h(
          "button",
          {
            className: "secondary-button",
            type: "button",
            onClick: () => onApiKeyChange(""),
          },
          "清除",
        ),
      ),
    ),
  );
}
