# Changelog

All notable changes to Typecrawl are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- A battle strip below the words: the profile's avatar sword-fights the
  current word, cutting a letter for each one typed correctly, while
  mistakes make the monster hit back. Special words are distinct monsters
  that drop their reward, active shields are drawn in front of the
  avatar, and a knockout is shown for a moment before the results (the
  pause isn't counted in the run's time). On by default; `b` in the menu
  turns it off per profile, and it hides itself in terminals smaller than
  70x16.

- Special words carry a marker as well as a colour, so they can be told
  apart without relying on colour: `*` golden, `+` green, `~` blue. The
  marker stays on the word while it is being typed, and the shield
  indicators carry the marker of the word that grants them.
- Views that used colour as a scale keep the scale without it. With
  colour off, the activity heatmap uses denser glyphs for busier days and
  the ghost curve on the results graph is faint.

### Changed

- The weak-key keyboard ranks keys by text attribute as well as colour,
  whether colour is on or off: plain, bold, bold and underlined, then
  reverse video for the most-missed keys. Keys never missed are gray, or
  faint with colour off.
- The ghost's entry in the results graph legend uses a dashed line.

### Fixed

- In quotes mode, a quote with a letter outside ASCII, such as "é", could
  not be typed correctly: the right key counted as a mistake and cost HP,
  and every letter after it in the word was checked against the wrong
  one. Backspace could also leave half a letter behind. Letters are now
  compared one by one, and words with them count the right number of
  characters towards WPM.
- Quotes with typographic punctuation (curly quotes, dashes, ellipses)
  now use the plain ASCII a keyboard can type: `'`, `"`, `-` and `...`.
  Invisible characters that made a word untypeable are removed, and a
  quote ending in a closing quote mark no longer gets an extra period.
- With `NO_COLOR` or `CLICOLOR=0` set, the typing view lost bold, underline and faint text
  along with the colours, so typed, mistyped and upcoming text all looked
  the same. Colour is now removed on its own: typed text is bold, mistakes
  are in reverse video, and upcoming words are faint.

## [0.4.0] - 2026-11-15

### Added

- `--import <file>` restores a profile from a file written by `--export`.
  Add `--profile NAME` to import it under a different name. Import never
  overwrites or merges into an existing profile, and an invalid file changes
  nothing.

### Changed

- `--export` now writes a complete backup. As well as the profile, test
  history and personal bests, the file contains settings, daily activity,
  weak-key counts, daily challenge claims, and the word source of each run,
  plus a format `version` field. Files exported by earlier versions can still
  be imported; they lack settings, weak-key counts and challenge claims, and
  their quotes-mode runs are imported as random-words runs.
- In the export file, an empty `history` or `personal_bests` is written as
  `[]` instead of `null`.

### Fixed

- Run history, personal bests and daily challenge claims could not be read
  back in time zones whose abbreviation is a four-digit offset, such as
  Nepal (`+0545`) and Iran (`+0330`). Timestamps are now stored in a form
  that reads back in every zone, and rows already saved in the unreadable
  form are repaired the next time the app starts. No data is lost.

## [0.3.0] - 2026-11-15

### Added

- Blue word: when a 10-word streak milestone lands in a run with no mistakes at
  all so far, the upcoming special word is blue instead of green. Typing it
  clean starts a 5-second shield during which every mistake is forgiven (no HP
  loss, no streak break) without the shield being spent.
  - Any wrong keystroke disqualifies the run, including one that was corrected
    or absorbed by the golden shield.
  - A slip before reaching a pending blue word turns it back into a regular
    green heal.
  - Words submitted with typos during the shield window do not count as clean,
    so the window cannot be used to mash through words for streak XP.
  - The timed shield is checked before the golden one, so a held golden shield
    survives until the timer runs out.

## [0.2.4] - 2026-10-21

### Changed

- Release binaries are built with `-s -w -trimpath`, shrinking them from about
  16 MB to about 11 MB with no functional change.
- The HP-bar colour styles are created once instead of on every render,
  removing an allocation from the 10×/second typing-view tick.

## [0.2.3] - 2026-10-20

### Changed

- Softened the mistake-combo damage curve. The combo window is halved
  (2 s → 1 s) and the multiplier now grows by +0.5 per hit, capped at 3×,
  instead of growing linearly without a cap. A burst of three rapid mistakes
  now costs 90 HP instead of 120, so it no longer knocks out a level-1
  character from full health.

## [0.2.2] - 2026-10-20

### Added

- A `tc` shortcut binary ships alongside `typecrawl` in every distribution
  channel: release tarballs, `.deb`/`.rpm` packages, and the Homebrew formula.

## [0.2.1] - 2026-10-20

### Changed

- Redesigned the profile page into bordered cards: the avatar/stats panel, both
  trend graphs (now side by side), the activity heatmap, and the week/month
  summary are each boxed.
- The achievement count moved into the stats card. "Almost there" and "Weakest
  keys" are now their own cards next to it, with weak keys rendered as
  individual keycaps.
- Added a separator before the heatmap's footer line.

### Added

- Demo GIF in the README.

## [0.2.0] - 2026-10-02

### Added

- Quotes mode: a new word source that uses whole quotes instead of random
  words, for both time and word tests. Toggle it with `s` in the menu or the
  `--quotes` flag.
  - Quotes are fetched from the DummyJSON quotes API and cached locally, so
    only the first quotes-mode launch needs the network. If that fetch fails
    with nothing cached, a small built-in quote list is used.
  - Quotes mode always implies punctuation.
  - `--focus-weak` cannot be combined with `--quotes`; the menu disables that
    toggle while quotes mode is active.
- Bookworm achievement line for quotes-mode runs, with six tiers at
  5/25/50/100/250/500 runs: Bookworm, Avid Reader, Bibliophile, Well-Read,
  Literary Scholar, Living Library.
- Grammarian expanded from a single achievement into the same six tiers:
  Grammarian, Punctilious, Stickler, Pedant, Linguist, Grammar Sage.

### Changed

- Personal bests and the ghost race are now tracked separately per word source,
  so a quotes run and a random-words run with the same mode and target no
  longer share a record. Existing databases are migrated automatically.
- Test history records the word source of each run.
- Achievement descriptions line up in a consistent column in the list view.

### Fixed

- Turning quotes mode on in the menu no longer leaves punctuation stuck on
  after quotes mode is turned back off.
- Quotes-mode runs no longer count toward the Grammarian line as well as the
  Bookworm line; punctuation runs are now counted for random-words runs only.

## [0.1.4] - 2026-10-01

### Added

- `-v`/`--version` flag that prints the build version (`dev` for local builds).
  `-h`/`--help` prints usage.

## [0.1.3] - 2026-10-01

### Added

- Golden word: a rare word (about 15% of exercises) that grants a one-shot
  shield if typed clean. The shield fully forgives the next mistake (no HP
  loss, no streak break).
- Green word: appears after every 10th consecutive mistake-free word and heals
  a flat amount of HP if typed clean. It can trigger again at later streak
  milestones in the same run.

### Fixed

- The green word is now marked two words ahead, so it is visible as an
  upcoming word. Previously it only became green at the moment it became the
  current word and its colour was never shown, although the heal still applied.

## [0.1.2] - 2026-10-01

### Fixed

- "Complete X exercises" achievements no longer count knocked-out runs, so a KO
  on a first exercise does not unlock "First Steps". KOs keep their own
  achievement line.

## [0.1.1] - 2026-10-01

### Changed

- On Linux the database now lives in the XDG data directory
  (`$XDG_DATA_HOME`, default `~/.local/share`) instead of the config directory.
  macOS and Windows are unchanged.

### Fixed

- Backspace in the profile-name field removes a whole character instead of a
  single byte, so accented, CJK, and emoji characters no longer leave a broken
  name behind.

### Security

- Profile names reject control characters and are capped in length, so a
  pasted ANSI/OSC escape sequence cannot inject terminal escape codes.

### Internal

- Added a test suite covering every package, a CI workflow that runs it on
  every push and pull request, and a test gate in the release workflow.
- Added a GitHub Actions workflow that runs GoReleaser on every `v*` tag.

## [0.1.0] - 2026-10-01

### Added

- Initial release: a terminal typing test built with Bubble Tea, with RPG-style
  progression.
  - XP and levelling, HP, and mistake combos.
  - Achievements and daily challenges.
  - Personal bests with a ghost-pace race.
  - Profile stats: WPM and accuracy trends, weak-key heatmap, and test history.
- GoReleaser configuration for the Homebrew tap and `.deb`/`.rpm` packages.

[Unreleased]: https://github.com/jmatelli/typecrawl/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/jmatelli/typecrawl/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/jmatelli/typecrawl/compare/v0.2.4...v0.3.0
[0.2.4]: https://github.com/jmatelli/typecrawl/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/jmatelli/typecrawl/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/jmatelli/typecrawl/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/jmatelli/typecrawl/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/jmatelli/typecrawl/compare/v0.1.4...v0.2.0
[0.1.4]: https://github.com/jmatelli/typecrawl/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/jmatelli/typecrawl/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/jmatelli/typecrawl/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/jmatelli/typecrawl/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/jmatelli/typecrawl/releases/tag/v0.1.0
