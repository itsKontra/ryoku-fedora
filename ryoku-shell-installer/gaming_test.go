package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gamingEventText(events chan any) string {
	close(events)
	var lines []string
	for event := range events {
		if line, ok := event.(evLine); ok {
			lines = append(lines, line.line)
		}
	}
	return strings.Join(lines, "\n")
}

func TestArchGamingWithCachyosUsesCatalogueAndNvidiaModule(t *testing.T) {
	edition := sourceEditionChoices("Arch")
	p := defaultPlanForEdition(&facts{}, edition)
	p.setComponent("cachyos-layer", true)
	availability, ok := edition.choices.Component("cachyos-layer")
	if !ok {
		t.Fatal("cachyos-layer is missing")
	}
	wantPackages := "linux-cachyos linux-cachyos-headers cachyos-settings scx-scheds cachyos-gaming-meta steam"
	if got := strings.Join(availability.Component.Packages, " "); got != wantPackages {
		t.Fatalf("CachyOS transaction packages = %q, want %q", got, wantPackages)
	}

	e := &engine{
		f:            &facts{distro: archLinux},
		p:            p,
		dry:          true,
		events:       make(chan any, 16),
		installedPkg: func(pkg string) bool { return pkg == "nvidia-open" },
	}
	if err := stepCachyosLayer(e); err != nil {
		t.Fatal(err)
	}
	output := strings.ReplaceAll(gamingEventText(e.events), "\n", " ")
	wantTransaction := "sudo -n pacman -Syu --needed --noconfirm " + wantPackages
	if !strings.Contains(output, wantTransaction) {
		t.Fatalf("CachyOS transaction missing from %q", output)
	}
	wantNvidia := "sudo -n pacman -S --needed --noconfirm linux-cachyos-nvidia-open"
	if !strings.Contains(output, wantNvidia) {
		t.Fatalf("NVIDIA parity transaction missing from %q", output)
	}
}

func TestCachyosNvidiaModuleFailureWarnsAndContinues(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	e := &engine{
		f:            &facts{distro: archLinux},
		events:       make(chan any, 8),
		installedPkg: func(pkg string) bool { return pkg == "nvidia-open" },
	}
	installCachyosNvidiaModule(e)
	output := gamingEventText(e.events)
	for _, want := range []string{
		"pacman -S --needed --noconfirm linux-cachyos-nvidia-open",
		"the CachyOS kernel has no NVIDIA module",
		"boot the Arch kernel",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("NVIDIA failure output missing %q: %s", want, output)
		}
	}
}

func TestSelectedGamingPackagesUseEditionPackageManager(t *testing.T) {
	t.Run("CachyOS", func(t *testing.T) {
		edition := sourceEditionChoices("CachyOS")
		p := defaultPlanForEdition(&facts{}, edition)
		p.setComponent("cachyos-gaming", true)
		e := &engine{
			f:       &facts{distro: archLinux},
			p:       p,
			payload: filepath.Clean(".."),
			dry:     true,
			events:  make(chan any, 32),
		}
		if err := stepPackages(e); err != nil {
			t.Fatal(err)
		}
		output := strings.ReplaceAll(gamingEventText(e.events), "\n", " ")
		for _, want := range []string{"pacman -Syu", " steam ", " cachyos-gaming-meta "} {
			if !strings.Contains(" "+output+" ", want) {
				t.Errorf("CachyOS package transaction missing %q: %s", strings.TrimSpace(want), output)
			}
		}
	})

	t.Run("Void", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join("..", "void", "packages", "translations.tsv"))
		if err != nil {
			t.Fatal(err)
		}
		edition, err := parseEditionCompositors(data)
		if err != nil {
			t.Fatal(err)
		}
		p := defaultPlanForEdition(&facts{}, edition)
		p.setComponent("void-gaming", true)
		e := &engine{
			f:       &facts{distro: voidLinux},
			p:       p,
			payload: filepath.Clean(".."),
			dry:     true,
			events:  make(chan any, 32),
		}
		if err := stepPackages(e); err != nil {
			t.Fatal(err)
		}
		output := strings.ReplaceAll(gamingEventText(e.events), "\n", " ")
		for _, want := range []string{
			"void-repo-multilib", "void-repo-nonfree", "steam", "mesa-dri-32bit",
			"vulkan-loader-32bit", "libgcc-32bit", "libstdc++-32bit", "libglvnd-32bit",
			"libdrm-32bit", "libva-32bit",
		} {
			if !strings.Contains(" "+output+" ", " "+want+" ") {
				t.Errorf("Void package transaction missing %s: %s", want, output)
			}
		}
		if strings.Index(output, "void-repo-nonfree") > strings.Index(output, " steam") {
			t.Fatalf("Void nonfree repository was not enabled before Steam: %s", output)
		}
	})
}
