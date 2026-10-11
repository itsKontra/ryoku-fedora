import QtQuick
import QtQuick.Layouts
import stage.modules.common
import stage.modules.common.widgets
import stage.services
import shell.services as ShellServices

StyledFlickable {
    id: root

    contentHeight: column.implicitHeight
    clip: true

    readonly property string design: ShellServices.Dock.design
    function cfg(key, fallback) { return ShellServices.Dock.cfg(key, fallback) }
    function setCfg(key, value) { ShellServices.Dock.setCfg(key, value) }

    ColumnLayout {
        id: column
        width: root.width
        spacing: 4

        EditDockDesignPicker {
            currentValue: root.design
            onSelected: value => root.setCfg("design", value)
        }

        EditPanelSectionLabel { text: Translation.tr("Shared placement") }

        EditPanelRow {
            Layout.fillWidth: true
            first: true
            last: false
            symbol: "dock"
            title: Translation.tr("Show dock")
            subtitle: Translation.tr("Uses this design on every monitor")
            trailingKind: "switch"
            switchChecked: root.cfg("enabled", false)
            rowEnabled: root.design !== "none"
            onActivated: root.setCfg("enabled", !root.cfg("enabled", false))
        }

        EditOptionChips {
            Layout.fillWidth: true
            label: Translation.tr("Screen edge")
            currentValue: root.cfg("edge", "auto")
            options: [
                { displayName: Translation.tr("Auto"), icon: "auto_awesome", value: "auto" },
                { displayName: Translation.tr("Bottom"), icon: "border_bottom", value: "bottom" },
                { displayName: Translation.tr("Top"), icon: "border_top", value: "top" },
                { displayName: Translation.tr("Left"), icon: "border_left", value: "left" },
                { displayName: Translation.tr("Right"), icon: "border_right", value: "right" }
            ]
            onSelected: value => root.setCfg("edge", value)
        }

        EditPanelRow {
            Layout.fillWidth: true
            first: false
            last: false
            symbol: "aspect_ratio"
            title: Translation.tr("Icon size")
            trailingKind: "stepper"
            valueText: Math.round(Number(root.cfg("size", 44))) + " px"
            stepDownEnabled: Number(root.cfg("size", 44)) > 28
            stepUpEnabled: Number(root.cfg("size", 44)) < 72
            onStepDown: root.setCfg("size", Math.max(28, Number(root.cfg("size", 44)) - 2))
            onStepUp: root.setCfg("size", Math.min(72, Number(root.cfg("size", 44)) + 2))
        }

        EditPanelRow {
            Layout.fillWidth: true
            first: false
            last: false
            symbol: "visibility_off"
            title: Translation.tr("Automatically hide")
            trailingKind: "switch"
            switchChecked: root.cfg("autohide", true)
            onActivated: root.setCfg("autohide", !root.cfg("autohide", true))
        }

        EditPanelRow {
            Layout.fillWidth: true
            first: false
            last: true
            symbol: "zoom_out_map"
            title: Translation.tr("Magnify on hover")
            trailingKind: "switch"
            switchChecked: root.cfg("magnify", true)
            onActivated: root.setCfg("magnify", !root.cfg("magnify", true))
        }

        EditPanelSectionLabel {
            visible: root.design === "ryoku"
            text: Translation.tr("Ryoku design")
        }

        EditOptionChips {
            visible: root.design === "ryoku"
            Layout.fillWidth: true
            label: Translation.tr("Shape")
            currentValue: root.cfg("style", "islands")
            options: ShellServices.Dock.styleOptions.map(option => ({
                displayName: option.label, value: option.key, icon: option.key === "rail" ? "view_week" : "dock"
            }))
            onSelected: value => root.setCfg("style", value)
        }

        Repeater {
            model: root.design === "ryoku" ? [
                { key: "frost", title: Translation.tr("Frosted surface"), symbol: "blur_on", fallback: true },
                { key: "shadow", title: Translation.tr("Depth shadow"), symbol: "layers", fallback: true },
                { key: "labels", title: Translation.tr("Hover labels"), symbol: "label", fallback: true },
                { key: "media", title: Translation.tr("Now playing card"), symbol: "music_note", fallback: false }
            ] : []
            delegate: EditPanelRow {
                required property var modelData
                required property int index
                Layout.fillWidth: true
                first: index === 0
                last: index === 3
                symbol: modelData.symbol
                title: modelData.title
                trailingKind: "switch"
                switchChecked: root.cfg(modelData.key, modelData.fallback)
                onActivated: root.setCfg(modelData.key, !root.cfg(modelData.key, modelData.fallback))
            }
        }

        Item {
            Layout.fillWidth: true
            implicitHeight: 8
        }
    }
}
