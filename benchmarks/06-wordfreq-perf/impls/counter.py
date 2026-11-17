# Counts with collections.Counter.
import collections, json, sys
counts = collections.Counter(open(sys.argv[1]).read().split())
top = sorted(counts.items(), key=lambda c: (-c[1], c[0]))[:10]
print(json.dumps([[w, n] for w, n in top]))
