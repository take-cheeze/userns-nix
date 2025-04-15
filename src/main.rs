use itertools::join;
use nix::unistd;
use nix::sched;
use nix::sys::stat;
use shell_escape;
use std;
use std::env;
use std::ffi::CString;
use std::fs;
use std::path::Path;
use sys_mount;

const START_SCRIPT: &str = r#"
set -x
nix_init_script="$XDG_STATE_HOME/nix/profile/etc/profile.d/nix.sh"
if [ ! -f "${nix_init_script}" ] ; then
	sh <(curl -L https://nixos.org/nix/install) --no-daemon
fi

. "${nix_init_script}"
"#;


fn main() -> std::io::Result<()> {
    let wd = env::current_dir()?;
    let uid = unistd::getuid();
    let gid = unistd::getgid();

	let config_dir = Path::new(&env::home_dir().ok_or(std::io::Error::other("unknown home dir"))?).join(".userns-nix");
	let user_root = config_dir.join("roots").join("root.".to_owned()+&std::process::id().to_string());
    fs::create_dir_all(&user_root)?;
    let nix_root = config_dir.join("nix");
    fs::create_dir_all(&nix_root)?;

	let xdg_state = config_dir.join("xdg-state");
	fs::create_dir_all(&xdg_state)?;

    let cmds = if env::args().len() > 1 {
		join(env::args().skip(1).map(|v| shell_escape::escape(v.into())), " ")
	} else {
        env::var("SHELL").map_err(|e| std::io::Error::other(e.to_string()))?
    };

    let mut clone_flags = sched::CloneFlags::empty();
    if uid.as_raw() != 0 {
        clone_flags |= sched::CloneFlags::CLONE_NEWUSER;
    }
    nix::sched::unshare(sched::CloneFlags::CLONE_NEWNS | clone_flags)?;

    if uid.as_raw() != 0 {
        fs::write("/proc/self/uid_map", format!("{} {} 1\n", uid.as_raw(), uid.as_raw()))?;
        fs::write("/proc/self/setgroups", format!("deny\n"))?;
        fs::write("/proc/self/gid_map", format!("{} {} 1\n", gid.as_raw(), gid.as_raw()))?;
    }

    sys_mount::Mount::builder()
        .fstype(sys_mount::FilesystemType::Manual("tmpfs"))
        .mount("none", &user_root)?;
    let bind_flags = sys_mount::MountFlags::BIND | sys_mount::MountFlags::REC;

    let user_nix_root = user_root.join("nix");
    unistd::mkdir(&user_nix_root, stat::Mode::S_IRWXU)?;
    sys_mount::Mount::builder()
        .fstype(sys_mount::FilesystemType::Manual("bind"))
        .flags(bind_flags)
        .mount(&nix_root, &user_nix_root)?;

    for e in fs::read_dir("/")? {
        let orig = e?.path();
        if !orig.is_dir() {
            continue
        }
        if let Some(f) = orig.file_name() {
            let bind_dir = user_root.join(f);
            if bind_dir.is_dir() {
                continue
            }
            unistd::mkdir(&bind_dir, stat::Mode::S_IRWXU)?;
            // println!("{} {}", bind_dir.display(), orig.display());
            sys_mount::Mount::builder()
                .fstype(sys_mount::FilesystemType::Manual("bind"))
                .flags(bind_flags)
                .mount(orig, bind_dir)?;
        }
    }

    unsafe {
        env::set_var("XDG_STATE_HOME", &xdg_state);
        // Check it with `nix config show | grep xdg`
        env::set_var("NIX_CONFIG", "use-xdg-base-directories = true\n");
    }

    unistd::chroot(&user_root)?;
    unistd::chdir(&wd)?;
 
    let args = &[
        CString::new("/bin/bash").unwrap(),
        CString::new("-c").unwrap(),
        CString::new(START_SCRIPT.to_owned()+"\n"+&cmds).unwrap()
    ];
    let _ = unistd::execv(&args[0], args);
    Ok(())
}