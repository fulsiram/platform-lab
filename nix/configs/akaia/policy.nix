{
  lib,
  pkgs,
  ...
}:
{
  fileSystems."/secrets/yggdrasil" = {
    fsType = "virtiofs";
    device = "yggdrasil-secret";
  };

  fileSystems."/secrets/ssh-keys" = {
    fsType = "virtiofs";
    device = "ssh-authorized-keys";
  };

  services.openssh.authorizedKeysFiles = [
    "/etc/ssh/authorized_keys.d/%u"
    "%h/.ssh/authorized_keys"
    "/var/lib/injected-keys/%u"
  ];

  systemd.services.initialize-ssh-keys = {
    before = [ "sshd.service" ];
    after = [ "secrets-ssh-keys.mount" ];
    wantedBy = [ "multi-user.target" ];

    unitConfig.RequiresMountsFor = [ "/secrets/ssh-keys" ];

    serviceConfig.Type = "oneshot";
    path = with pkgs; [ util-linux ];
    script = ''
      install -d -m 755 /var/lib/injected-keys
      install -m 644 /secrets/ssh-keys/authorized_keys /var/lib/injected-keys/user
    '';
  };

  services.yggdrasil = {
    persistentKeys = false;
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
  };

  environment.systemPackages = with pkgs; [
    git
  ];

  environment.persistence."/persistent" = {
    enable = true;
    hideMounts = true;
  };

  security.sudo.wheelNeedsPassword = false;
}
