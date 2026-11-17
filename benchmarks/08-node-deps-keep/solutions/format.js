const padleft = require("padleft");

// price(1250) is "$12.50": cents as dollars with two decimals.
function price(cents) {
  return "$" + (cents / 100).toFixed(2);
}

// slug("Hello, World!") is "hello-world": lower case, runs of other characters become one dash, no dash at either end.
function slug(text) {
  return text.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
}

// initials("ada lovelace") is "AL": the first letter of each word, upper case.
function initials(name) {
  return name.split(/\s+/).filter(Boolean).map(w => w[0].toUpperCase()).join("");
}

// truncate("abcdefgh", 5) is "abcd…": at most max characters, the last one an ellipsis when text was cut.
function truncate(text, max) {
  return text.length > max ? text.slice(0, max - 1) + "…" : text;
}

// clock(7, 5) is "07:05": hours and minutes, each two digits.
function clock(hours, minutes) {
  return padleft(hours, 2, "0") + ":" + padleft(minutes, 2, "0");
}

module.exports = { price, slug, initials, truncate, clock };
