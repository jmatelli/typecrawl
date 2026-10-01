# Typecrawl

A terminal-based typing test, inspired by [monkeytype](https://monkeytype.com), with an RPG-style
progression system layered on top: XP and leveling, an HP/combo mechanic for mistakes, achievements,
daily challenges, and a "ghost" race against your own personal best.

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

On first run you'll be prompted to create a profile. From there:

- **Menu**: `tab`/`m` switch mode, `←`/`→` or `h`/`l` change duration/word count, `c`/`z`/`f` toggle
  punctuation/zen/focus-weak, `enter` starts, `p` opens your profile, `u` switches profile.
- **Typing**: type the words; `backspace` corrects mistakes (even across completed words); `esc` quits
  back to the menu.
- **Profile**: `a` achievements, `h` test history, `k` weak-key heatmap, `t` cycles an equipped title,
  `r` reset, `d` delete.

Quick-launch flags skip straight to a test:

```sh
typecrawl --profile Alice --time 30 --punctuation
typecrawl --list-profiles
typecrawl --profile Alice --export backup.json
```

## License

MIT, see [LICENSE](LICENSE).
