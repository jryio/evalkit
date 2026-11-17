# Makes the fixture's history: five releases, one commit each.
set -eu
n=1
for feature in login search export billing audit; do
  printf 'RELEASE = %s\nFEATURE = "%s"\n\nif __name__ == "__main__":\n    print("release %%d: %%s" %% (RELEASE, FEATURE))\n' "$n" "$feature" > app.py
  git add -A
  git commit -q -m "release $n: $feature"
  n=$((n + 1))
done
