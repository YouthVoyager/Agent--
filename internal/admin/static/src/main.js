import { App } from "./App.js";
import { React, createRoot, h } from "./lib/react.js";

createRoot(document.getElementById("root")).render(
  h(
    React.StrictMode,
    null,
    h(App),
  ),
);
