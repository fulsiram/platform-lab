{
  pkgs,
  config,
  ...
}:
{
  imports = [
    ./hardware-configuration.nix
    ./disk-config.nix
    ./boot.nix
    ./networking.nix
  ];

  sops.secrets.ssh_public_key = {
    neededForUsers = true;
  };

  services.openssh.enable = true;
  services.k3s.enable = true;
  services.k3s.extraFlags = [
    "--cluster-init"
    "--tls-san=pepperoni.salami.network"
  ];
  services.k3s.disable = [ "traefik" ];

  environment.systemPackages = with pkgs; [
    curl
    wget
    git
    htop
    tmux
  ];

  users.users.admin = {
    isNormalUser = true;
    extraGroups = [ "wheel" ];
    openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN6+2C1xsDgmYV/9JahsUeSU++WvegOznRgO9/Qd+Msg fulsiram@mbp"
    ];
  };

  security.sudo.wheelNeedsPassword = false;

  system.stateVersion = "25.11";
}
