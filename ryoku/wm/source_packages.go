package wm

// SourcePackages is the runtime needed before a source-built provider can run.
// Distribution package-name translation belongs to the installer.
func SourcePackages(provider string) []string {
	switch provider {
	case ProviderHyprland:
		return []string{"hyprland", "hyprpolkitagent", "xdg-desktop-portal-hyprland", "hypridle", "hyprpicker"}
	case ProviderNiri:
		return []string{"niri", "xwayland-satellite", "xdg-desktop-portal-gnome"}
	}
	return nil
}
