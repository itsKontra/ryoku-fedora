pragma ComponentBehavior: Bound
import QtQuick
import "Singletons"

// The visualiser stack without its own surface: the desktop hosts one at
// the visualizer's scene z while parallax owns the background, so the
// bars sit between the parallax layers.
Item {
    id: root

    Repeater {
        // A look being aimed stays drawn even when the policy unloads a silent
        // or power-saving visualizer: an empty grip places nothing.
        model: (Performance.visualizerResident || Spectrum.placementHolds > 0) && Config.enabled
            ? Config.count : 0
        delegate: VisualizerView {
            id: vizView
            anchors.fill: parent
            cfg: VizItem { data: Config.dataAt(vizView.index) }
        }
    }
}
