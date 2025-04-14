package cgo

/*
// Originally from https://github.com/howardjohn/unshare-go
#cgo CFLAGS: -Wall
#define _GNU_SOURCE
#include <sched.h>
#include <stdlib.h>
#include <unistd.h>
#include <stdio.h>
#include <string.h>

int uid = 0;
int gid = 0;

__attribute((constructor)) void enter_userns(void) {
	uid = getuid();
	gid = getgid();
	int f = CLONE_NEWNS;
	if (uid != 0) {
		f |= CLONE_NEWUSER;
		puts("with user namespace!\n");
	}
	if (unshare(f) < 0) {
		perror(strerror(f));
		puts("unshare fail!\n");
		exit(1);
	}
	puts("unshare success!\n");

	return;
}
*/
import "C"

func Uid() int {
	return int(C.uid)
}

func Gid() int {
	return int(C.gid)
}
