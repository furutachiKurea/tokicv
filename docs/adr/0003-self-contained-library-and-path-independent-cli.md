# Self-Contained Application Library and Path-Independent CLI

Status: accepted

Both GUI and CLI initialize `mygo.App.SetName("tokicv")` before accessing `mygo.PathUserData`, storing internal Resume Documents, Layout Profiles, and per-file overrides inside that self-contained directory without coupling to external note vaults. Opening an external `.md` path edits and saves it in place with optional per-path Layout Profile overrides, importing copies it into the library, and exporting writes PDF/PNG/Markdown Artifacts to the chosen destination; CLI `export` and `check` accept any relative or absolute `.md` path (as well as library document names/IDs) with explicit CLI flag > document override > global default precedence.
