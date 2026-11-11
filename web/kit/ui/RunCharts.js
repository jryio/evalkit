// The Runs view's charts, drawn with Observable Plot. RunTrends is the top row (the
// score run by run, and the score against spend); RunPairs is the selected run against
// its comparison, item by item.
//
// Props, shared by both:
//   suite     the full suite (dimensions, groups, runs)
//   state     the review state; judgeOnly and group are the view's filters
//   rows      summarize(suite, state, { judgeOnly, group }) in run order
//   metric    metricOf(suite); spend: spendOf(suite) or null
//   runs      runList(suite)
//   selected  the selected run id; vs: the run it is compared with (a runs entry), or null
//   select    (runId) => void: select a run
//   open      (runId, { pair, direction }) => void: open the Table on runId, with the
//             item runId/pair selected, or filtered to items that went "up" | "same" | "down"
export function RunTrends() {
  return null;
}

export function RunPairs() {
  return null;
}
