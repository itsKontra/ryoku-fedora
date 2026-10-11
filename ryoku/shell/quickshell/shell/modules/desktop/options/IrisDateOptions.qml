pragma ComponentBehavior: Bound
import QtQuick
import ".."
import "../Singletons"
import Ryoku.Ui.Singletons

// Face options for the vendored iRiS Date badge face.
IrisOptionsBase {
    id: opts

    MenuSection { label: I18n.tr("Date"); gloss: "日付" }
    MenuRow {
        label: I18n.tr("Style")
        value: opts.cap(String(opts.g("style", "ticket")))
        closeOnTrigger: false
        onTriggered: opts.put("style", opts.cycle(["ticket", "stacked", "seal", "instrument"], String(opts.g("style", "ticket"))))
    }
    MenuRow {
        label: I18n.tr("Weekday")
        value: opts.g("showWeekday", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showWeekday", true); closeOnTrigger: false
        onTriggered: opts.toggle("showWeekday", true)
    }
    MenuRow {
        label: I18n.tr("Year")
        value: opts.g("showYear", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showYear", true); closeOnTrigger: false
        onTriggered: opts.toggle("showYear", true)
    }
    MenuRow {
        label: I18n.tr("Day of the year")
        value: opts.g("showOrdinal", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("showOrdinal", true); closeOnTrigger: false
        onTriggered: opts.toggle("showOrdinal", true)
    }
    MenuRow {
        visible: String(opts.g("style", "ticket")) === "instrument"
        label: I18n.tr("Registration marks")
        value: opts.g("instrumentMarks", true) ? I18n.tr("On") : I18n.tr("Off")
        on: opts.g("instrumentMarks", true); closeOnTrigger: false
        onTriggered: opts.toggle("instrumentMarks", true)
    }
}
