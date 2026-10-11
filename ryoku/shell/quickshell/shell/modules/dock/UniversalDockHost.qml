import QtQuick
import shell.services

// One per monitor: loads the selected dock design (Ryoku's islands, or none)
// whatever the bar style, so an unselected design never creates a window.
Item {
    id: root

    required property var screen
    property bool surfaceVisible: true
    readonly property string design: Dock.design
    readonly property var item: root.design === "ryoku" ? designLoader.item?.surface : null

    width: 0
    height: 0

    Loader {
        id: designLoader
        active: root.surfaceVisible && Dock.cfg("enabled", false) && root.design !== "none"
        asynchronous: true
        sourceComponent: ryokuDesign
    }

    Component {
        id: ryokuDesign
        Item {
            property alias surface: dockSurface
            width: 0
            height: 0
            DockSurface {
                id: dockSurface
                screen: root.screen
                visible: root.surfaceVisible && Dock.cfg("enabled", false)
            }
        }
    }
}
