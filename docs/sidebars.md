# Controls

`Super+Escape` opens **Controls** at the top-left of the focused display, under
every bar style and on both Hyprland and niri. It closes with Escape, its close
button, or a click outside. The shortcut is editable in **Ryoku Hub > Keybinds**,
and `ryoku-shell quicksettings` opens it from a script or a niri bind. The bar's
quick-settings menu is unchanged and still opens from its rail button.

Controls is a compact, configurable 688-logical-pixel panel.

## Vitals

The hero is a compact system monitor. It shows the user and host, uptime and
load average, total CPU load with a per-core grid, CPU frequency and
temperature, memory and swap, GPU load, temperature and VRAM, network traffic,
disk traffic with root usage and drive temperature, and battery state. Missing
sensors are omitted rather than shown as zero.

`SystemMonitor` samples four times a second on a worker thread while the panel
is active; loads and transfer rates are measured over a sliding one-second
window, and slow sensors (temperatures, battery, disk usage) refresh once a
second. `SystemGraph` draws bounded native histories for CPU, memory, GPU,
network, disk, and temperatures over a one-minute window, keeps a second more
than it shows so the oldest sample is always beyond the left edge, and eases
each new sample in over one sample period so the trace never steps. Sampling
and graph animation stop when the panel is inactive. Hiding Vitals prevents its
monitor from being created; hiding the live graph prevents its graph renderer
from being created.

## Connections and power profile

Wi-Fi and Bluetooth tiles show connection state at a glance and open their
device lists in the same panel. Ethernet and VPN appear as presence chips when
available. The power profile switch offers only the profiles
power-profiles-daemon reports, follows changes made elsewhere, and reads
unavailable when the daemon is missing.

## Levels

Volume and brightness are fine sliders with direct value entry and small step
controls. Expanding Volume opens an inline drawer for output devices, playback
apps, recording apps, microphones, mute controls, and default-device selection.
Expanding Brightness opens a drawer with one control per display. Changing a
level does not silently change its mute state.

## Bottom bar

Lock, Sleep, Log out, Restart, and Power off are hold-to-activate actions. Hold a
button until its fill completes; releasing early cancels it. Keyboard users can
hold Space or Enter. The completed hold is the confirmation, so there is no
second dialog. Lock runs `ryoku-shell lock` and Sleep runs `ryoku-shell
suspend`, the same commands as Super+L and the lid; Log out, Restart, and Power
off go through the daemon's `session.*` calls.

The same bar carries Night light when supported, Keep awake, Do not disturb,
Mic mute, and Gaming mode when supported. The gear opens a small popup for the
panel's Motion speed (Quick, Standard, Calm: how fast it opens, closes, and
reveals its sections) and shortcuts into Ryoku Hub. Unsupported controls are not
shown as dead options.

## Customising Controls

**Ryoku Hub > Controls** has a miniature of the panel rather than a list of
switches. Drag Vitals, Connections, Power profile, Now playing, Levels, and
Bottom controls into the order you want. The eye on each block shows or hides
the whole block. Selecting a block exposes chips for its smaller parts,
including individual readings, the live graph, connection tiles, level sliders,
media details and transport, session actions, and quick controls.

Now playing is available but hidden until selected. **Reset** restores the
shipped arrangement. The choice is stored in `shell.json` under `controls`
(the daemon validates it and drops unknown ids), applies immediately to an open
panel on every display, and is used the next time it opens. Hidden blocks are
not instantiated, so their probes, processes, watches, and animations do not
continue in the background.

## Contributor map

Controls components live under `ryoku/shell/quickshell/shell/modules/sidebar/`
(see its README). Shared state is owned by
`shell/services/SidebarState.qml`. Native sampling and rendering live in the
`Ryoku.Blobs` plugin: `ryoku/shell/plugin/systemmonitor.{hpp,cpp}` and
`ryoku/shell/plugin/systemgraph.{hpp,cpp}`. The plugin also carries
`thinkingorb` and `borderbeam`, the native pieces the upstream Ask bar draws
with.
