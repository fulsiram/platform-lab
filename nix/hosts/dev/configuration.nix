{
  modulesPath,
  pkgs,
  ...
}:
{
  imports = [
    "${modulesPath}/profiles/qemu-guest.nix"
    ./disk-config.nix
  ];

  boot.growPartition = true;
  boot.kernelParams = [ "console=ttyS0" ];
  boot.loader.timeout = 0;

  boot.loader.grub.efiSupport = true;
  boot.loader.grub.efiInstallAsRemovable = true;
  boot.loader.grub.device = "/dev/vda";

  services.qemuGuest.enable = true;
  services.openssh.enable = true;
  services.cloud-init.enable = true;
  systemd.services."serial-getty@ttyS0".enable = true;

  systemd.services.grow-partitions = {
    wantedBy = [ "multi-user.target" ];
    after = [ "nix.mount" ];
    requires = [ "nix.mount" ];

    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      SuccessExitStatus = "0 1";
    };

    path = with pkgs; [
      cloud-utils.guest
      e2fsprogs
    ];

    script = ''
      growpart /dev/disk/by-id/virtio-NIXSTORE 1 || true;
      resize2fs /dev/disk/by-id/virtio-NIXSTORE-part1
    '';
  };

  fileSystems."/secrets/yggdrasil" = {
    fsType = "virtiofs";
    device = "yggdrasil-secret";
  };

  services.yggdrasil = {
    enable = true;
    settings.Peers = [
      # akaia.org
      "tls://185.195.236.220:42137"
      "tls://sto01.yggdrasil.hosted-by.skhron.eu:8884"
    ];
    settings.PrivateKeyPath = "/secrets/yggdrasil/privateKey.pem";
  };

  systemd.services.yggdrasil = {
    requires = [ "secrets-yggdrasil.mount" ];
    after = [ "secrets-yggdrasil.mount" ];
  };

  users.users.user = {
    isNormalUser = true;
    extraGroups = [ "wheel" ];
    openssh.authorizedKeys.keys = [
      "sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAIDHnobcDBE+4+AOdYZj2tjQcplRfazs0YAdNePw2MLkfAAAAEXNzaDpwZXJzb25hbC1hdXRo fulsiram@personal-yk-1-auth"
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGQ5mp97prL9Yvy1ZlfaEIyPaODbroGSBpzMqRANbpsv shadowfox@akaia.org"
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAjv/S+sOP+W+PUQBLpJFfczqWFfgzeWczaw0wszMrbK shadowfox@san-ti-hub"
    ];
  };

  security.sudo.wheelNeedsPassword = false;
}
