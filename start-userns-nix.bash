#!/usr/bin/env bash

set -eux -o pipefail

user_root=${HOME}/.userns-nix/root.$$
nix_root=${HOME}/.userns-nix/nix

mkdir -p "${nix_root}"

unshare -Um --map-root-user -- bash - <<EOS

set -eux -o pipefail

mkdir -p "${user_root}"
mount -ttmpfs none "${user_root}"
mkdir "${user_root}/nix"
mount --bind "${nix_root}" "${user_root}/nix"

for i in /* ; do
    bind_dir="${user_root}/\$(basename \${i})"
    if [ -d "\${bind_dir}" ] || [ ! -d "\${i}" ] ; then
        continue
    fi
    mkdir -p "\${bind_dir}"
    mount -R "\${i}" "\${bind_dir}"
done

prev_wd=$PWD

cat /proc/self/uid_map
cat /proc/self/gid_map

# --userspec "$(id -u):$(id -g)" 
/sbin/chroot "${user_root}"

set -eux -o pipefail

if [ ! -d "/nix/var/nix/profiles/default/etc/profile.d/nix.sh" ] ; then
    sh <(curl -L https://nixos.org/nix/install) --no-daemon
fi

cd "\${prev_wd}"

. /nix/var/nix/profiles/default/etc/profile.d/nix.sh

exec ${SHELL}

EOS

rmdir "${user_root}"
