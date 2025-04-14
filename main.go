package main

// go fmt && go build . && ./userns-nix

/*
#cgo CFLAGS: -Wall
#define _GNU_SOURCE
#include <sched.h>
#include <stdlib.h>
#include <unistd.h>

int uid = 0;
int gid = 0;

__attribute((constructor(101))) void enter_userns(void) {
	uid = getuid();
	gid = getgid();
	int f = CLONE_NEWNS;
	if (uid != 0) {
		f |= CLONE_NEWUSER;
	}
    if (unshare(f) < 0) {
        exit(1);
    }
    return;
}
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

const startScript = `
# set -x
nix_init_script="$XDG_STATE_HOME/nix/profile/etc/profile.d/nix.sh"
if [ ! -f "${nix_init_script}" ] ; then
	# Workarouds for incomplete supports of $XDG_STATE_HOME in nix eco-systems
	ln -s $(readlink $HOME/.nix-profile) $HOME/.nix-profile.old

	mv ~/.local/state ~/.local/state.old

	mkdir -p ~/.local/state/nix/profiles

	mv ~/.nix-channels ~/.nix-channels.old

	cat >/tmp/nix-extra.conf <<EOS
use-xdg-base-directories = true
EOS
	sh <(curl -L https://nixos.org/nix/install) --no-daemon --nix-extra-conf-file /tmp/nix-extra.conf

	mv ~/.nix-channels.old ~/.nix-channels

	mv ~/.local/state/nix/profiles/* $XDG_STATE_HOME/nix/profiles
	rm -f $XDG_STATE_HOME/nix/profile
	ln -s $XDG_STATE_HOME/nix/profiles/profile $XDG_STATE_HOME/nix/profile

	rm -rf ~/.local/state
	mv ~/.local/state.old ~/.local/state

	mv ~/.nix-profile $XDG_STATE_HOME/nix/profile
	ln -s $(readlink $HOME/.nix-profile.old) $HOME/.nix-profile
	rm -f $HOME/.nix-profile.old
fi

. "${nix_init_script}"
`

func bindMound(orig string, bind string) {
	err := os.Mkdir(bind, 0770)
	if err != nil && !os.IsExist(err) {
		log.Panicf("mount point(%s) creation failed: %s", bind, err)
	}
	err = syscall.Mount(orig, bind, "bind", syscall.MS_REC|syscall.MS_BIND, "")
	if err != nil {
		log.Panicf("mount %s to %s failed: %s", orig, bind, err)
	}
}

func main() {
	wd, err := os.Getwd()
	if err != nil {
		log.Panicf("%s", err)
	}

	// log.Printf("original uid: %d gid: %d", C.uid, C.uid)
	// log.Printf("current uid: %d gid: %d", os.Getuid(), os.Getgid())

	if os.Getuid() != 0 {
		err := os.WriteFile("/proc/self/uid_map", []byte(fmt.Sprintf("%d %d 1\n", C.uid, C.uid)), 0640)
		if err != nil {
			log.Panicf("failed user map: %s", err)
		}
		err = os.WriteFile("/proc/self/setgroups", []byte("deny"), 0640)
		if err != nil {
			log.Panicf("failed setgroups: %s", err)
		}
		err = os.WriteFile("/proc/self/gid_map", []byte(fmt.Sprintf("%d %d 1\n", C.gid, C.gid)), 0640)
		if err != nil {
			log.Panicf("failed group map: %s", err)
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Panicf("%s", err)
	}
	configDir := filepath.Join(home, ".userns-nix")
	userRoot := filepath.Join(configDir, "roots", "root."+strconv.Itoa(os.Getpid()))
	err = os.MkdirAll(userRoot, 0770)
	if err != nil {
		log.Panicf("%s", err)
	}
	/*
		// TODO: Fix resource busy
		defer func() {
			err := syscall.Unmount(userRoot, 0)
			if err != nil {
				log.Panicf("%s", err)
			}
			err = os.Remove(userRoot)
			if err != nil {
				log.Panicf("%s", err)
			}
		}()
	*/

	nixRoot := filepath.Join(configDir, "nix")
	err = os.MkdirAll(nixRoot, 0770)
	if err != nil {
		log.Panicf("%s", err)
	}

	err = syscall.Mount("none", userRoot, "tmpfs", 0, "")
	if err != nil {
		log.Panicf("%s", err)
	}
	bindMound(nixRoot, filepath.Join(userRoot, "nix"))

	files, err := os.ReadDir("/")
	if err != nil {
		log.Panicf("%s", err)
	}
	for _, file := range files {
		orig := "/" + file.Name()
		origInfo, err := os.Stat(orig)
		if !origInfo.IsDir() {
			continue
		}
		bindDir := filepath.Join(userRoot, file.Name())
		_, err = os.Stat(bindDir)
		if err == nil {
			continue
		}
		bindMound(orig, bindDir)
	}

	xdgState := filepath.Join(configDir, "xdg-state")
	os.Setenv("XDG_STATE_HOME", xdgState)
	err = os.MkdirAll(xdgState, 0770)
	if err != nil {
		log.Panicf("%s", err)
	}

	err = syscall.Chroot(userRoot)
	if err != nil {
		log.Panicf("chroot failed: %s", err)
	}

	err = os.Chdir(wd)
	if err != nil {
		log.Panicf("chdir failed: %s", err)
	}

	cmd := exec.Command("bash", "-c", startScript+"\n"+os.Getenv("SHELL"))
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err = cmd.Run()
	if err != nil {
		log.Panicf("%s", err)
		os.Exit(err.(*exec.ExitError).ExitCode())
	}
}
