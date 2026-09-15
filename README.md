# playlist-linter

A linter for M3U/M3U8 audio playlists. Playlists get hand-edited, merged
from other tools, or exported by software with its own idea of what a
valid entry looks like, and the result is files that "work" in one player
and silently drop tracks in another. This tool reads a playlist and prints
every problem it finds along with the line number, so you can go fix the
file directly instead of guessing which line broke it.

## Usage

```
go build -o playlint .
./playlint mytape.m3u
```

Given a playlist like:

```
#EXTINF:180,Side A - Track 1
songs/track1.mp3
#EXTINF:abc,Side A - Track 2
songs/track2.mp3
songs/track1.mp3
#EXTINF:200,Side A - Track 4
cover.jpg
```

it reports:

```
mytape.m3u:1: warning missing-header: playlist does not start with #EXTM3U
mytape.m3u:3: error bad-duration: duration "abc" is not a number
mytape.m3u:5: warning duplicate-track: "songs/track1.mp3" was already listed at line 2
mytape.m3u:7: warning unknown-extension: "cover.jpg" does not look like an audio file
```

Exit status is 1 if any finding is an error, 2 if a file couldn't be read,
0 otherwise.

## Rules

- `missing-header` - the file doesn't start with `#EXTM3U`.
- `bad-duration` - an `#EXTINF` duration isn't a number, or is negative
  without being the documented `-1` "unknown length" sentinel used for
  live streams.
- `dangling-extinf` - an `#EXTINF` line isn't followed by a track path.
- `duplicate-track` - the same path or URL appears more than once.
- `unknown-extension` - a local path's extension isn't a recognized audio
  format. URLs are exempt, since query strings and streaming endpoints
  don't always end in a file extension.
- `trailing-whitespace` - a track line has trailing spaces or tabs. Some
  players include that whitespace literally when resolving the path, so
  the file "exists" but never plays.
- `backslash-path` - a local path uses backslashes. That's a Windows path
  separator; most other players won't resolve it. URLs are exempt.

## Multiple files

Pass as many playlist paths as you like; each is linted independently and
findings are prefixed with the file path they came from.

## Status

Early. The rule set above covers the mistakes I've actually hit in my own
playlists; see the issues for what's planned next.
