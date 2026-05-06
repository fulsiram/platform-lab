{ modulesPath, ... }:
{
  imports = [
    "${modulesPath}/virtualisation/kubevirt.nix"
  ];

  services.openssh.enable = true;

  users.users.user = {
    isNormalUser = true;
    extraGroups = [ "wheel" ];
    openssh.authorizedKeys.keys = [
      "sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAIDHnobcDBE+4+AOdYZj2tjQcplRfazs0YAdNePw2MLkfAAAAEXNzaDpwZXJzb25hbC1hdXRo fulsiram@personal-yk-1-auth"
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAjv/S+sOP+W+PUQBLpJFfczqWFfgzeWczaw0wszMrbK shadowfox@san-ti-hub"
    ];
  };

  security.sudo.wheelNeedsPassword = false;
}
