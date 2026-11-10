// One import for components: html`` templates (htm bound to Preact) and the hooks.
import { h, render } from "preact";
import htm from "htm";

export const html = htm.bind(h);
export { render };
export { useState, useEffect, useLayoutEffect, useRef, useMemo } from "preact/hooks";
