// Reads the API's user record on stdin and prints one line per field.
let input = "";
process.stdin.on("data", d => (input += d));
process.stdin.on("end", () => {
  const u = JSON.parse(input);
  console.log("name: " + u.user_name);
  console.log("since: " + u.created);
  console.log("spent: " + u.total);
  console.log("admin: " + (u.is_admin ? "yes" : "no"));
  console.log("address: " + u.addr);
});
