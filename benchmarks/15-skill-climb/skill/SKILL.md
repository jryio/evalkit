---
name: changelog
description: Add an entry to CHANGELOG.md. Use whenever a change needs a changelog entry.
---

# Changelog

When asked for a changelog entry, add it to CHANGELOG.md.

## Where the entry goes

- Put the new entry in the `## Unreleased` section.
- Insert it as the first bullet under the `## Unreleased` heading, above any existing entries. Do not append it to the end of the section.

## Format

- Use the bullet format already used in CHANGELOG.md, with a bracketed kind tag such as `[added]`, `[changed]`, `[fixed]` or `[removed]`, followed by the change description.
- Choose the tag that matches the kind of change: a new capability is `[added]`, a bug fix is `[fixed]`, a behaviour change is `[changed]`, and a removal is `[removed]`.
- Check the existing entries for the tag vocabulary in use before writing a new tag.
- Keep each entry to one line of at most 90 characters, counting the leading `- `, the tag and the description together. If the description runs longer, shorten it (drop filler words, keep the what and the effect) rather than wrapping it onto a second line. Count the characters before finishing.
