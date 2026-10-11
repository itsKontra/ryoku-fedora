pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS News ticker face.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("News"); gloss: "報道" }
    MenuRow {
        label: I18n.tr("Style")
        value: opts.cap(String(opts.g("style", "card")))
        closeOnTrigger: false
        onTriggered: opts.put("style", opts.cycle(["card", "instrument"], String(opts.g("style", "card"))))
    }
    MenuRow {
        label: I18n.tr("Source and time")
        value: opts.g("showMeta", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showMeta", true); closeOnTrigger: false
        onTriggered: opts.toggle("showMeta", true)
    }
}
