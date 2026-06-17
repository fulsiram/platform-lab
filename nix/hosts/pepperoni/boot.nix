{ lib, ... }:
{
  boot = {
    zfs.forceImportRoot = false;

    loader = {
      grub = {
        enable = true;
        # efiSupport = true;
        # efiInstallAsRemovable = true;

        mirroredBoots = [
          {
            path = "/boot0";
            # efiSysMountPoint = "/boot0";
            # devices = [ "nodev" ];
            devices = [ "/dev/disk/by-id/nvme-SAMSUNG_MZQLB1T9HAJR-00007_S439NC0R700843" ];
          }
          {
            path = "/boot1";
            # efiSysMountPoint = "/boot1";
            # devices = [ "nodev" ];
            devices = [ "/dev/disk/by-id/nvme-SAMSUNG_MZQLB1T9HAJR-00007_S439NC0R700977" ];
          }
        ];
      };
    };

    kernelParams = [ "ip=162.55.238.111::162.55.238.65:255.255.255.255:::none" ];
    initrd = {
      # luks.devices.p1.device = lib.mkForce "/dev/disk/by-partlabel/crypt_p1";
      # luks.devices.p2.device = lib.mkForce "/dev/disk/by-partlabel/crypt_p2";

      # luks.reusePassphrases = true;

      availableKernelModules = [
        "igb"
        # Hetzner QEMU vKVM
        "virtio_pci"
        "virtio_blk"
        "virtio_net"
        "e1000e"
      ];

      network = {
        enable = true;
        ssh = {
          enable = true;
          port = 2222;
          hostKeys = [
            "/etc/secrets/initrd/ssh_host_ed25519_key"
            "/etc/secrets/initrd/ssh_host_rsa_key"
          ];
          authorizedKeys = [
            "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN6+2C1xsDgmYV/9JahsUeSU++WvegOznRgO9/Qd+Msg fulsiram@mbp"
          ];
          # shell = "/bin/cryptsetup-askpass";
        };
      };
    };
  };

  boot.loader.grub.devices = lib.mkForce [ ];
}
