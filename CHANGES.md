[DiffNav Extra](https://github.com/lukaszcz/diffnav-extra) is a fork of [DiffNav](https://github.com/dlvhdr/diffnav) with extra features and bespoke modifications.

Changes with respect to upstream DiffNav:
- light theme support,
- select & copy to clipboard in the diff view window,
- OSC 52 clipboard instead of atotto/clipboard,
- line wrapping in delta unified diff view,
- `Enter` on a file opens the file in editor (same as `o`),
- `Ctrl+↑/↓` scroll the diff by one line, `PgUp`/`PgDn` scroll by a page, `Home`/`End` scroll to the top/bottom of the diff view window,
- remember scroll position in the diff view window,
- toggle directories on file tree icon click,
- fix blank diff panes caused by reusing incomplete cache entries after rapid file switching.
