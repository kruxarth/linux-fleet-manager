import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
// Self-hosted fonts (no CDN) so the console works offline.
import "@fontsource/fredoka/latin-500.css";
import "@fontsource/fredoka/latin-600.css";
import "@fontsource/nunito/latin-500.css";
import "@fontsource/nunito/latin-700.css";
import "@fontsource/nunito/latin-800.css";
import App from "./App";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
