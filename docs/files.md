# Files (tab 2) and Projects (tab 3)

## Files

The files of the SD card, the unit's own storage, or this computer (`t` changes the source). It is
where samples come from: pick a file, assign it to a pad.

The view is a **tree** of folders, or (`w`) **three columns** like a file manager.

| Key | What |
| --- | --- |
| `j k`, arrows, `g` `G` | move |
| `Tab` / `l` | open a folder (in three columns: go into it; `Enter` too) |
| `h` | close it / go to the parent |
| `Shift-Tab` | fold every folder |
| `{` `}` (`Alt-p` `Alt-n`) | previous / next folder in the list |
| `w` | switch between the tree and three columns |
| `t` | next source: SD card, unit storage, this computer |
| `[` `]` | choose the target **bank** for assigns |
| `Space` | listen to the file on this computer |
| `Ctrl-A` | autoplay: a file plays as the cursor rests on it |
| `Enter` | assign the file to a pad: a picker opens on the target bank's next free pad |
| `Z` | slice the file in the **Slicer** |

### Choosing a pad (picker)

Type a bank letter and a pad number (`C`, `7`, `Enter`); `A`–`J` bank, `1`–`16` pad, arrows move,
`[ ]` bank, `Enter` choose, `Esc` back.

### Marking, copying, deleting (SD card)

| Key | What |
| --- | --- |
| `v` | mark a file or folder (`Enter` then fills free pads in order) |
| `V` | mark every file in the folder |
| `J` | mark macOS / Windows junk (`._` files, `.DS_Store`) |
| `u` | unmark everything |
| `d` | delete the marked (SD card only) |
| `c` / `x` | copy / cut the marked to the clipboard |
| `p` | paste into the folder in front (also from another source) |
| `N` / `n` | new folder / rename |
| `R` | restore pads from this folder's export (asks which bank) |

### Search, tags and the index

| Key | What |
| --- | --- |
| `/` | search every audio file in the source by keyword: `Enter` assigns, `Tab` shows it in the tree |
| `s` (in a search) | search tags only |
| `f` | find a folder by name among the indexed and open folders |
| `i` | index the folder in front in the background (tags from file and folder names, saved to `SD:404tui-index.json.gz`); again stops it |
| `*` | favourite: tag the marked files (or the one in front); again takes it off |
| `+` | tag the marked files with words you type (a word with `-` in front takes the tag off) |

## Projects (tab 3)

The unit's projects. Keys: `Enter` select the project, `n` rename, `e` export it (SD card or this
computer), `R` restore from a backup folder, `X` erase it (asks first), `/` find by name.

## Working offline

`sp404 -offline <folder>` works on a folder of exported projects (each a folder with `PADCONF.BIN`)
without a unit. `U` leaves offline mode when a unit is plugged in.
