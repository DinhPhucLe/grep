# Anonymous local participant profiles

`POST /participants` accepts `{"id":"<UUID v4>","display_name":"Ada"}`.
The optional name is at most 100 characters; ID must be canonical lowercase UUID
v4. Requests are bounded to 4 KiB and unknown fields are rejected. There is no
login, password, email requirement, authorization header, or account session.

MongoDB's `participants` collection stores `_id`, optional `display_name`,
`created_at`, and `last_seen_at`. Atomic registration preserves the creation
timestamp and existing name when the name is omitted. `last_seen_at` records the
latest registration; the TUI registers before each prompt evaluation.

The TUI stores this ID at `os.UserConfigDir()/cortisol/participant.json`. New files
are private and published atomically so simultaneous TUI launches reuse one ID.
Corrupt files cause an actionable error rather than silently losing identity.

The ID identifies a profile, not a verified person or access credential. Profiles
are separate from the older `users` collection; no existing user records change.

Migration `000008_participant_answers` creates `participants` and `quiz_answers`
with validators and answer uniqueness/history indexes. Applying it requires
explicit approval. Registration fails with HTTP 503 `migration_required` if the
collection is absent; it never creates a collection implicitly.
