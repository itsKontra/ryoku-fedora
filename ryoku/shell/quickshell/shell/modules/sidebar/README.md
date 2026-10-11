# Sidebar module

Controls is the compact top-left panel shared by every bar style and compositor.

- `Sidebar.qml` owns placement, the content window, and click-away input.
- `SidebarFrame.qml` and `SidebarChrome.qml` provide the shared surface and header.
- `ControlsBoard.qml` renders the persisted section order and instantiates only
  the blocks the user has chosen to show.
- `ControlsHero.qml` and `Vital*.qml` present native system activity.
- `ControlsConnections.qml` and `ConnectionTile.qml` present Wi-Fi, Bluetooth,
  Ethernet, and VPN state.
- `PowerProfileControl.qml` switches the power-profiles-daemon profile.
- `ControlsLevels.qml`, `LevelSlider.qml`, `MixerDrawer.qml`, `MixerRow.qml`, and
  `BrightnessDrawer.qml` provide fine audio and per-display brightness controls.
- `ControlsMedia.qml` is the optional now-playing block.
- `ControlsBar.qml`, `HoldButton.qml`, and `ControlsSettingsPopup.qml` own the
  hold-to-activate session actions, quick toggles, and gear popup.
- `cards/SystemWifiPage.qml` and `cards/SystemBluetoothPage.qml` are the
  built-in detail pages.
- `shell/services/SidebarState.qml` owns per-display Controls state and routing.

The root shell loads the panel asynchronously and unloads it after closing.
Gate live work on the panel's active state. `shell.json` `controls` stores
section order, section visibility, and hidden sub-elements. Keep its shipped
defaults aligned with `SidebarState.qml`, the daemon settings contract
(`ryoku/shell/ipc/settings.go`), and the Hub Controls schema
(`ryoku/hub/quickshell/schema/ControlsPage.js`). A hidden block must not be
loaded or keep a probe, timer, file watch, or animation alive.

See [`docs/sidebars.md`](../../../../../../docs/sidebars.md) for behavior.
