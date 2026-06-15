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

  environment.etc."rancher/k3s/authentication.yaml".text = ''
    apiVersion: apiserver.config.k8s.io/v1
    kind: AuthenticationConfiguration
    jwt:
    - issuer:
        url: https://auth.salami.network/realms/members
        audiences:
        - kube-apiserver
      claimMappings:
        username:
          claim: email
          prefix: "oidc:"
        groups:
          claim: groups
          prefix: "oidc:"
        uid:
          claim: sub
      userValidationRules:
      - expression: "!user.username.startsWith('system:')"
        message: "username cannot use reserved system: prefix"
      - expression: "user.groups.all(g, !g.startsWith('system:'))"
        message: "groups cannot use reserved system: prefix"
  '';

  services.k3s.enable = true;
  services.k3s.extraFlags = [
    "--cluster-init"
    "--flannel-backend=none"
    "--disable-kube-proxy"
    "--disable-network-policy"
    "--tls-san=pepperoni.salami.network"
    "--cluster-cidr=10.42.0.0/16,fdd2:3f8b:6035:1::/56"
    "--service-cidr=10.43.0.0/16,fdd2:3f8b:6035:2::/112"
    "--kube-apiserver-arg=authentication-config=/etc/rancher/k3s/authentication.yaml"
  ];

  services.k3s.disable = [
    "traefik"
    "servicelb"
  ];

  environment.systemPackages = with pkgs; [
    curl
    wget
    git
    htop
    tmux
    tcpdump
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
