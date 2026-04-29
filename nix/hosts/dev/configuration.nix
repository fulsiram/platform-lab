{ ... }:
{
  services.openssh.enable = true;

  users.users.user = {
    isNormalUser = true;
    extraGroups = [ "wheel" ];
    openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAso7x0jGW6tCRrV++d07ooI+lxyE1YZTAR7iYfjhHnh"
      "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDIACwwdSZSTjnPdksTRamfbY41sgrzKpE3IQ8A2mvN/FG7ktjNmUx1MwM4h6AMs3/6cfu2dBRwnVrsGflhXZgtcGYgvh/ffjp0zStSkRBSIXUzWd8qb7wZa+3lJFaJ1zzlQpniWxTSYlNSF0zQTBctcRdJrCVDrlnFekCMPCjXJwloY5skhIcv3iFGHTfIZLHnNlwtQw7wX0/x+bdRBtEm61MfE99jpsfe62Pkf/6fNVF5c2uy7I9pqqz+obUx9xagy6/uHrNsUiJudrhuJMPNm5QL9vIrgFQ0MDPqwUUdMZHYJ1rS7zLw0az96uBzwl10zwehWdUDRk3DtaNHIUdNMp15fb8/osykS8rBwR/U1ofnwIlct2wMI5qXA0aDU9c140AoqNlndFrZIyptUalKqeFAG2FfMO1jW0jKpgkxT4oOxRu/lwq6POU2PSShfOF104eYdNRV4TitQVLiZfImlbdnK7xzhrWUjLjMpISRG4md3IGLVzW9SR+T/s2eOTCFNv0xBAwWlSslquZBr2rGe3V8PoawdywbUs3E5/q8+G6UK7Wju6NpMSuGRdtQb0ETBAQN09uFB06TULJXvedwYuTO8KmVjpdrO4fAoXtlAp1n3ZMNWfDMfW85auMaefYpHaNuOSM9U4MGlSWsecJRCEv6c2SWmP9i7TfAGNqo0w=="
    ];
  };

  security.sudo.wheelNeedsPassword = false;
}
