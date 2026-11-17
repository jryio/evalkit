# Counts in one pass with a dictionary.
import json, sys
counts = {}
for w in open(sys.argv[1]).read().split():
    counts[w] = counts.get(w, 0) + 1
top = sorted(counts.items(), key=lambda c: (-c[1], c[0]))[:10]
print(json.dumps([[w, n] for w, n in top]))
