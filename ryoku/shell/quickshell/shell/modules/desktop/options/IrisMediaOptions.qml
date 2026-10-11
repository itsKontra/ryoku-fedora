pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS Now Playing face: the player layout, synced
// lyrics, and the built-in visualiser. Lyrics reads through irisOption(); the
// visualiser knobs are the face's own raw keys.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Media"); gloss: "音楽" }
    MenuRow {
        label: I18n.tr("Layout")
        value: opts.cap(String(opts.g("playerPreset", "full")))
        closeOnTrigger: false
        onTriggered: opts.put("playerPreset", opts.cycle(["full", "compact", "minimal"], String(opts.g("playerPreset", "full"))))
    }
    MenuRow {
        label: I18n.tr("Synced lyrics")
        value: opts.g("iris.lyrics", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("iris.lyrics", true); closeOnTrigger: false
        onTriggered: opts.toggle("iris.lyrics", true)
    }

    MenuSection { label: I18n.tr("Visualiser"); gloss: "音波" }
    MenuRow {
        label: I18n.tr("Type")
        value: opts.cap(String(opts.g("visualizerType", "wave")))
        closeOnTrigger: false
        onTriggered: opts.put("visualizerType", opts.cycle(["wave", "bars", "organic"], String(opts.g("visualizerType", "wave"))))
    }
    MenuRow {
        label: I18n.tr("Placement")
        value: opts.cap(String(opts.g("visualizerPosition", "bottom")))
        closeOnTrigger: false
        onTriggered: opts.put("visualizerPosition", opts.cycle(["bottom", "top", "fill", "none"], String(opts.g("visualizerPosition", "bottom"))))
    }
    MenuSlider {
        id: vizOp
        label: I18n.tr("Visualiser opacity")
        from: 0; to: 100; step: 1
        value: Number(opts.g("visualizerOpacity", 55))
        valueText: Math.round(vizOp.value) + "%"
        onMoved: (v) => opts.put("visualizerOpacity", Math.round(v))
        onReleased: (v) => opts.put("visualizerOpacity", Math.round(v))
    }
}
