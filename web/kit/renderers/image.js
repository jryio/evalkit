// An image with an optional caption. Suite-relative paths load from /files/; a click
// opens the full image in a new tab.
import { html } from "../html.js";

const srcOf = s => (/^(https?:\/\/|\/)/.test(s) ? s : `/files/${s}`);

export default function ImageArtifact({ a }) {
  const src = srcOf(a.src);
  return html`<figure class="image">
    <a href=${src} target="_blank" rel="noreferrer"><img src=${src} alt=${a.alt ?? ""} /></a>
    ${a.caption ? html`<figcaption>${a.caption}</figcaption>` : null}
  </figure>`;
}
