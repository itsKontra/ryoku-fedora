pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS Notes face (distinct from Ryoku's native
// Notes pad): the note text, its presentation and typography.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Notes"); gloss: "筆記" }
    MenuTextField {
        label: I18n.tr("Text")
        placeholder: I18n.tr("Note text")
        text: String(opts.g("text", ""))
        onCommitted: (v) => opts.put("text", v)
    }
    MenuRow {
        label: I18n.tr("Style")
        value: opts.cap(String(opts.g("style", "card")))
        closeOnTrigger: false
        onTriggered: opts.put("style", opts.cycle(["card", "instrument"], String(opts.g("style", "card"))))
    }
    MenuRow {
        label: I18n.tr("Font")
        value: opts.cap(String(opts.g("fontFamily", "sans")))
        closeOnTrigger: false
        onTriggered: opts.put("fontFamily", opts.cycle(["sans", "mono"], String(opts.g("fontFamily", "sans"))))
    }
    MenuRow {
        label: I18n.tr("Alignment")
        value: opts.cap(String(opts.g("textAlign", "left")))
        closeOnTrigger: false
        onTriggered: opts.put("textAlign", opts.cycle(["left", "center", "right"], String(opts.g("textAlign", "left"))))
    }
    MenuSlider {
        id: fs
        label: I18n.tr("Font size")
        from: 10; to: 28; step: 1
        value: Number(opts.g("fontSize", 14))
        valueText: Math.round(fs.value)
        onMoved: (v) => opts.put("fontSize", Math.round(v))
        onReleased: (v) => opts.put("fontSize", Math.round(v))
    }
    MenuRow {
        label: I18n.tr("Ruled lines")
        value: opts.g("showRules", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showRules", true); closeOnTrigger: false
        onTriggered: opts.toggle("showRules", true)
    }
}
