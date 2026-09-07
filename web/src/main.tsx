import { createRoot } from "react-dom/client";
import App from "./App";
import { connect } from "./store";
import "./styles.css";

// The stream is opened outside React, because the store it feeds lives outside React. It stays open
// for the life of the page (ADR-0006 §6.1).
connect();

const root = document.getElementById("root");
if (root === null) {
  throw new Error("nothing to mount into: index.html has no #root");
}
createRoot(root).render(<App />);
