import { html, render } from "./html.js";
import "./builtins.js";
import { App } from "./ui/App.js";
import { Boundary } from "./ui/bits.js";

render(html`<${Boundary} label="The review page"><${App} /></${Boundary}>`, document.getElementById("app"));
