pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS World clock face, including its list of
// zones (a comma-separated IANA list, e.g. Asia/Tokyo, Europe/London).
IrisOptionsBase {
    id: opts

    readonly property var defaultZones: ["Asia/Tokyo", "Europe/London", "America/New_York"]


    function zonesText() {
        const z = opts.g("timezones", opts.defaultZones);
        return (Array.isArray(z) ? z : opts.defaultZones).join(", ");
    }
    function commitZones(v) {
        const arr = String(v).split(",").map(s => s.trim()).filter(s => s.length > 0);
        opts.put("timezones", arr);
    }

    MenuSection { label: I18n.tr("World clock"); gloss: "世界" }
    MenuRow {
        label: I18n.tr("Style")
        value: String(opts.g("style", "cards")) === "instrument" ? I18n.tr("Instrument") : I18n.tr("List")
        closeOnTrigger: false
        onTriggered: opts.put("style", opts.cycle(["cards", "instrument"], String(opts.g("style", "cards"))))
    }
    MenuRow {
        visible: String(opts.g("style", "cards")) === "instrument"
        label: I18n.tr("Layout")
        value: String(opts.g("instrumentLayout", "grid")) === "rows" ? I18n.tr("Strip") : I18n.tr("Atlas")
        closeOnTrigger: false
        onTriggered: opts.put("instrumentLayout", opts.cycle(["grid", "rows"], String(opts.g("instrumentLayout", "grid"))))
    }
    MenuRow {
        label: I18n.tr("City names")
        value: opts.g("showNames", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showNames", true); closeOnTrigger: false
        onTriggered: opts.toggle("showNames", true)
    }
    MenuRow {
        label: I18n.tr("Time difference")
        value: opts.g("showOffsets", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showOffsets", true); closeOnTrigger: false
        onTriggered: opts.toggle("showOffsets", true)
    }
    MenuRow {
        label: I18n.tr("Date")
        value: opts.g("showDate", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showDate", true); closeOnTrigger: false
        onTriggered: opts.toggle("showDate", true)
    }
    MenuRow {
        label: I18n.tr("Day / night")
        value: opts.g("showDayState", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showDayState", true); closeOnTrigger: false
        onTriggered: opts.toggle("showDayState", true)
    }

    MenuSection { label: I18n.tr("Cities"); gloss: "都市" }
    MenuTextField {
        label: I18n.tr("Time zones")
        placeholder: I18n.tr("Asia/Tokyo, Europe/London")
        text: opts.zonesText()
        onCommitted: (v) => opts.commitZones(v)
    }
}
