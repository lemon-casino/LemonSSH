# Migration Slice Record Template

Copy the section below into `../migration-ledger.md` after completing or
blocking a migration slice. Replace every angle-bracket field. Do not leave
placeholders in committed records.

```markdown
## WV3-L<NNN> - <YYYY-MM-DD> - <slice ID and title>

- Capability rows: `<ID>`, `<ID>`
- Plan task: `<implementation-plan reference>`
- Status change: `<previous> -> <new>`
- Scope change: `none`
- Goal: <observable replacement outcome>
- Go canonical owner: `<paths>`
- Frontend adapter: `<paths or none>`
- Electron owner affected: `<paths>`
- Preserved invariants: <short list>
- Data/schema impact: <migration, compatibility, or none>
- Security impact: <authority, permissions, secrets, containment, or none>
- Verification: `<commands, CI links, benchmarks, manual platform checks>`
- Platforms covered: `<Windows/macOS/Linux and architectures>`
- Evidence grade: `<A/B/C>`
- Decision references: `<decision IDs or none>`
- Gate: `none`
- Closure evidence: `none`
- Electron retirement: `deleted: <paths>`, `removed: <paths>`,
  `cutover-trigger: <single trigger and reason>`, or
  `rollback-trigger: <single trigger and reason>`; baseline-only records use
  `none: baseline task only`; blocked records may use
  `none: blocked evidence only`
- Documentation updated: `<matrix, plan, architecture, other owner docs>`
- Residual risks: <bounded unresolved risks or none>
- Next safe slice: `<ID/title>`
- Drift decision: `continue | pause-for-user | needs-verification | blocked`
```

Status transitions must be exact backtick values (`<previous> -> <new>`) that
follow the checker's transition graph; `migrated` additionally requires evidence
grade A and canonical retirement evidence. A scope removal uses exact
`<CAPABILITY-ID>: required -> removed`, transitions that same row to `retired`,
cites an accepted decision carrying exact category
`scope-removal:<CAPABILITY-ID>`, and records `removed:` or `deleted:` evidence in
that entry. An unrelated decision does not authorize removal.

NONAI gate records use this exact field syntax and do not advance or apply a
transition to the mixed capability states:

```markdown
- Capability rows: all non-AI required rows
- Plan task: `P6-05`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Verification: nonAiRows=verified; releaseTargets=<accepted release-target decision IDs>; agentDecisions=<accepted Agent decision IDs>; qualification=P6-02,P6-03,P6-04; authority=<nonempty>
- Evidence grade: `A`
- Decision references: `<accepted decision IDs collectively carrying every required NONAI category>`
- Gate: `NONAI-COMPLETE`
- Closure evidence: `none`
- Electron retirement: `none: Wails disconnected; Electron frozen release carrier until approved cutover/rollback triggers`
```

The referenced NONAI decisions must collectively carry all exact categories:
`release-target:windows`, `release-target:macos`, `release-target:linux`,
`agent-runtime:cursor-bun`, `agent-runtime:opencode-bun`,
`agent-disposition:copilot`, `agent-disposition:codebuddy`, and
`agent-disposition:cursor-cli`. Verification prose or arbitrary decision IDs do
not satisfy this requirement. `releaseTargets` and `agentDecisions` are
comma-separated `WV3-NNN` lists without spaces or duplicates. Every ID must be
accepted, category-relevant, and present in `Decision references`; the release
list carries all three release categories and the Agent list carries all five
Agent categories. A required row falling from `verified` or `migrated` below a
completed state invalidates the current epoch; `verified -> migrated` does not.
An approved post-gate scope removal invalidates the epoch that counted the row
as required; a later gate may exclude it. Production AI paths then become
forbidden again, and after recovery another NONAI record is required before AI
advancement.

The release lifecycle gate forms are:

```markdown
- Capability rows: `REL-03.1`
- Plan task: `P8-02`
- Status change: `verified -> verified`
- Scope change: `none`
- Verification: rc=<nonempty>; platforms=<nonempty>; migration=<nonempty>; rollback=<nonempty>; authority=<nonempty>
- Evidence grade: `A`
- Gate: `WAILS-CUTOVER`
- Closure evidence: `none`
```

```markdown
- Capability rows: `REL-03.2`
- Plan task: `P8-03`
- Status change: `not-started -> not-started`
- Scope change: `none`
- Decision references: `<accepted decision carrying rollback-window-closure>`
- Gate: `ROLLBACK-CLOSED`
- Closure evidence: `thresholds=<nonempty>; sample=<nonempty>; blockers=<nonempty>; authority=<nonempty>`
```

Gate status values must repeat the current row status and have no transition
effect. All ordinary and baseline records use `Gate: none`, `Scope change: none`
and `Closure evidence: none`. Every real transition to
`probe`, `implemented`, `verified`, or `migrated` needs `deleted:`, `removed:`,
`cutover-trigger:`, or `rollback-trigger:` evidence. No-op baseline/governance
records may use `none:`. A `blocked` transition needs a trigger or the exact
`none: blocked evidence only` marker. A `retired` transition needs `removed:` or
`deleted:` evidence and the same-entry capability-specific scope-removal decision.
Later no-op records with `none:` do not erase a previously valid trigger.
`REL-03.1` first advances only under P8-01. `REL-03.2` first advances only under
P9-01 after both release gates, and reaches `verified`/`migrated` only under
P9-02 with the accepted rollback-window closure decision.

Completion checklist:

- [ ] The implementation serves the approved migration goal.
- [ ] No duplicate owner or unplanned fallback was introduced.
- [ ] Required tests and platform evidence were read in full.
- [ ] The capability matrix status matches the evidence grade.
- [ ] The implementation plan reflects completion and the next action.
- [ ] Electron retirement is complete or has one precise trigger.
- [ ] Related architecture/current-owner docs are updated where behavior changed.
