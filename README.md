# Typecrawl

A terminal-based typing test, inspired by [monkeytype](https://monkeytype.com), with an RPG-style
progression system layered on top: XP and leveling, an HP/combo mechanic for mistakes, achievements,
daily challenges, and a "ghost" race against your own personal best.

![Typecrawl demo](docs/demo.gif)

## Install

### Homebrew (macOS/Linux)

```sh
brew install jmatelli/typecrawl/typecrawl
```

### Debian/Ubuntu (.deb)

Download the `.deb` for your architecture from the
[latest release](https://github.com/jmatelli/typecrawl/releases/latest) and install it:

```sh
sudo dpkg -i typecrawl_*_linux_amd64.deb
```

### Windows

Download the `windows_x86_64.zip` (or `windows_arm64.zip` on ARM) from the
[latest release](https://github.com/jmatelli/typecrawl/releases/latest), extract it, and run it from
Windows Terminal:

```powershell
Expand-Archive typecrawl_*_windows_x86_64.zip -DestinationPath $HOME\typecrawl
& $HOME\typecrawl\typecrawl.exe
```

To run `typecrawl` (or `tc`) from any folder, add that folder to your `PATH`. Profiles are stored in
`%AppData%\typecrawl`.

The binaries aren't code-signed, so the first run may show a SmartScreen warning: choose "More info",
then "Run anyway".

### Go install

```sh
go install github.com/jmatelli/typecrawl@latest
```

### From source

```sh
git clone https://github.com/jmatelli/typecrawl.git
cd typecrawl
go build -o typecrawl .
```

## Usage

```sh
typecrawl
```

`tc` ships alongside `typecrawl` as a shorter alias -- same binary, same flags.

On first run you'll be prompted to create a profile. From there:

- **Menu**: `tab`/`m` switch mode, `←`/`→` or `h`/`l` change duration/word count, `s` cycles the text
  source (random words, quotes, then your saved files), `x` forgets the selected file, `c`/`z`/`f` toggle
  punctuation/zen/focus-weak, `b` toggles the battle strip, `enter` starts, `p` opens your profile, `u`
  switches profile.
- **Typing**: type the words; `backspace` corrects mistakes (even across completed words); `esc` quits
  back to the menu.
- **Profile**: `a` achievements, `h` test history, `k` weak-key heatmap, `t` cycles an equipped title,
  `p` prestige (from level 30), `r` reset, `d` delete.
- **Weak keys**: `l` cycles the keyboard layout the heatmap is drawn in (QWERTY, AZERTY, QWERTZ,
  Dvorak, Colemak). The choice is saved per profile. Mistyped characters the layout has no key for,
  such as accented letters on QWERTY, are listed in an "other" row below the keyboard.
- **Results**: `enter` starts another run, `s` shares the run, `q` quits.

Quick-launch flags skip straight to a test:

```sh
typecrawl --profile Alice --time 30 --punctuation
typecrawl --list-profiles
typecrawl --profile Alice --export backup.json
```

### Your own text

Type from a file of your own instead of the built-in words: another language, code, numbers or symbols.

```sh
typecrawl --profile Alice --time 60 --wordlist ~/french-words.txt   # words drawn at random
typecrawl --profile Alice --words 200 --text ~/chapter-one.txt      # typed in order
```

- **Word list**: words separated by spaces or line breaks, drawn at random like the built-in words.
  Repeated words count once. `--punctuation` and `--focus-weak` work as usual.
- **Text**: typed in order from the start, as written, so punctuation is always on. A run longer than the
  text goes back to the beginning.
- **Files**: UTF-8 text of up to 4 MB. Accented letters, other alphabets and wide characters such as
  Japanese show and type correctly. Words are split on spaces, though, so text written without them
  becomes one long word. Curly quotes and dashes are turned into the keys you'd type, as in quotes mode.
- **Saved**: a file you launch with is saved to the profile, and the menu's `s` key offers it from then
  on. The file is read again at the start of every run, so edits show up. If it has been moved, the menu
  says so; `x` forgets it.
- **Rewards**: runs on your own text earn XP, and each file keeps its own personal bests and ghost.
  They don't count toward achievements, daily or weekly challenges, or the profile's exercise and
  knockout counts, because a list of very short words would make those trivial. They do count as
  practice for the daily streak.

### Sharing a run

Press `s` on the results screen to copy a short summary of the run to the clipboard:

```text
typecrawl · 62.4 WPM · 97.1% accuracy
time 30s · random words · punctuation
word streak 14 · level 7 · new personal best
```

The copy uses the terminal's OSC 52 clipboard sequence, so it also works over SSH, but some terminals
ignore it without any sign; inside tmux it needs `set -g set-clipboard on`. That's why every run
you shared is also printed when you quit.

For scripts, `--json` prints each run finished during the session as one JSON object per line on
standard output when the program exits. The game itself is drawn on standard error, so standard output
can be piped or redirected:

```sh
typecrawl --profile Alice --time 30 --json | jq .wpm
```

```json
{"profile":"alice","finished_at":"2026-11-15T18:04:05Z","mode":"time","target":30,"source":"random","punctuation":true,"zen":false,"wpm":62.43,"accuracy":97.1,"duration_seconds":30,"words_typed":31,"word_streak":14,"level":7,"prestige":0,"xp_gained":58,"new_personal_best":true,"ko":false,"verified":true}
```

| Field | Meaning |
| --- | --- |
| `profile` | profile name |
| `finished_at` | when the run ended, RFC 3339 |
| `mode`, `target` | `"time"` with a target in seconds, or `"words"` with a word count |
| `source` | `"random"` words, `"quotes"`, or your own `"wordlist"` or `"text"` |
| `source_name` | the word list's or text's file name, without its directory; absent for the others |
| `punctuation`, `zen` | whether those options were on |
| `wpm`, `accuracy` | words per minute and accuracy in percent, to two decimals |
| `duration_seconds` | how long the run took |
| `words_typed` | words completed |
| `word_streak` | longest run of mistake-free words |
| `level`, `xp_gained` | the profile's level after the run, and the XP it earned |
| `prestige` | the profile's prestige rank (0 until it first prestiges) |
| `new_personal_best` | whether the run set a personal best |
| `ko` | whether the run ended in a knockout (no XP) |
| `verified` | `false` for a run that looked scripted; it earned nothing and was not saved |

A session with no finished run prints nothing.

### Backup and restore

`--export` writes everything a profile owns to a JSON file: level and XP, test history, personal bests
and ghost runs, settings, the daily streak and activity heatmap, weak-key counts, and daily and weekly
challenge claims, and your saved word lists and texts. Those are saved by their full path, which
names your home folder, so the files themselves aren't in the backup. A backup with your own files
in it needs this version or later to import; older versions say it's a newer format. `--import` creates a new profile from that file, on the same machine or another one:

```sh
typecrawl --profile Alice --export backup.json
typecrawl --import backup.json
typecrawl --import backup.json --profile "Alice 2"   # import under a different name
```

Import never overwrites or merges into an existing profile: if the name is already taken, nothing is
imported. Files exported by v0.3.0 or earlier can still be imported, but they lack settings, weak-key
counts and daily challenge claims, and every run in them is imported as a random-words run, so progress
toward the quotes-mode achievements is not restored.

### Challenges

The menu shows two challenges, each paying a one-off XP bonus that the daily streak multiplier doesn't
apply to.

- **Daily**: one objective for a single run, the same for everyone on a given day. It's rated easy
  (+30 XP), medium (+50) or hard (+80). Some ask for a setup (quotes, punctuation, a 2-minute run), others
  for a result (accuracy, speed, a word streak, no hits taken) or for landing green, blue or other special
  words.
- **Weekly**: one goal reached over several runs from Monday to Sunday, such as 10 exercises, 1,000
  words or 4 different days of practice. The menu shows your progress, for example `4/10`. Progress comes
  from your run history.

A run counts toward neither challenge if it ended in a knockout or was slower than 10 WPM.

### Prestige

Word difficulty rises with your level: Medium words from level 10, Hard from 20, Expert from 30. Once a
profile reaches level 30, press `p` on the profile screen, then `y` to confirm. The profile goes back to
level 1 with no XP and gains a prestige rank, which makes every later climb harder: rank 1 starts on
Medium words, rank 2 on Hard, and rank 3 or higher on Expert, with the mistake damage of that tier and
only a level-1 profile's HP growth to absorb it.

Prestiging keeps everything else: run history, personal bests, streaks, achievements and your equipped
title. Prestige ranks 1, 3 and 5 each unlock an achievement you can wear as a title. Resetting a profile
also clears its prestige.

### Battle strip

Below the words, your profile's avatar fights the current word with a sword. Each correct letter cuts one
letter off the monster, and backspacing puts it back. A mistake makes the monster hit back, harder in a
combo. Special words are their own monsters and drop their reward when defeated cleanly. An active
shield is drawn in front of the avatar, and a knockout leaves it in pieces for a moment before the
results.

The strip is on by default. Press `b` in the menu to turn it off for the current profile. It also hides
itself when the terminal is smaller than 70x16, and in zen mode nothing hits back. It's only decoration:
the strip doesn't change scoring, and the pause after a knockout isn't counted in your time.

### Special words and colour

Three kinds of special word can appear in a run. Each has its own colour and a marker in front of it, so
it can be recognised without relying on the colour:

| Marker | Colour | Typed without a mistake                                                   |
| ------ | ------ | ------------------------------------------------------------------------- |
| `*`    | gold   | grants a shield that forgives the next mistake                            |
| `+`    | green  | restores HP                                                               |
| `~`    | blue   | starts a 5-second shield; appears instead of the green word in a flawless run |

The shield indicators next to the HP bar carry the same markers: `* shield active` and `~ shield 3.2s`.

Set the [`NO_COLOR`](https://no-color.org) environment variable (or `CLICOLOR=0`) to turn colour off
everywhere:

```sh
NO_COLOR=1 typecrawl
```

The typing view then uses bold for what you've typed, reverse video for mistakes, and faint text for
the words still to come. Views that use colour as a scale keep the scale another way: the activity
heatmap uses denser glyphs for busier days (`· ░ ▒ ▓ █`), the weak-key keyboard ranks keys from faint (never missed) through plain, bold, and bold underlined to
reverse video for the most-missed keys, and the ghost curve on the results graph is faint.

## License

MIT, see [LICENSE](LICENSE).
