import QtQuick
import shell.services
import "../stage/Singletons" as StageCfg

// One per monitor: loads the selected dock design (Ryoku's islands, or none)
// whatever the bar style, so an unselected design never creates a window.
Item {
    id: root

    required property var screen
    property bool surfaceVisible: true
    readonly property string design: Dock.design
    readonly property var item: root.design === "ryoku" ? designLoader.item?.surface : null
    // The Stage Editor frames this monitor: the dock stays mounted, revealed
    // and raised, and publishes the edge it occupies so the mode's viewport
    // shrinks around it. The dock catalogue edits the live surface, so hiding
    // it while that page is open edits an invisible thing.
    readonly property bool stageEditing: StageCfg.StageSession.onMonitor(
        root.screen ? root.screen.name : "")

    width: 0
    height: 0

    // Shima draws its own dock from the same store, so this one steps aside.
    Loader {
        id: designLoader
        active: root.surfaceVisible && Dock.cfg("enabled", false) && root.design !== "none"
            && Config.barStyle !== "iris"
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
                stageEditing: root.stageEditing
                visible: root.surfaceVisible && Dock.cfg("enabled", false)
            }
        }
    }
}
