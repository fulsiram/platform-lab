{
  lib,
  ...
}:
{
  fileSystems."/secrets/yggdrasil" = {
    fsType = "virtiofs";
    device = "yggdrasil-secret";
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
    openssh.authorizedKeys.keys = [
      "sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29tAAAAIDHnobcDBE+4+AOdYZj2tjQcplRfazs0YAdNePw2MLkfAAAAEXNzaDpwZXJzb25hbC1hdXRo fulsiram@personal-yk-1-auth"
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGQ5mp97prL9Yvy1ZlfaEIyPaODbroGSBpzMqRANbpsv shadowfox@akaia.org"
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAjv/S+sOP+W+PUQBLpJFfczqWFfgzeWczaw0wszMrbK shadowfox@san-ti-hub"
    ];
  };

  security.sudo.wheelNeedsPassword = false;
}
