{ self }:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.clerkd;
  settingsFormat = pkgs.formats.toml { };
  defaultSettings = {
    server.bind_to_address = [ "127.0.0.1:6601" ];
    mpd.address = "localhost:6600";
    random = {
      tracks = 20;
      artist_tag = "albumartist";
    };
    cache.batch_size = 10000;
  };
in
{
  options.services.clerkd = {
    enable = lib.mkEnableOption "Clerk daemon";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.clerkd;
      defaultText = lib.literalExpression ''self.packages.''${pkgs.stdenv.hostPlatform.system}.clerkd'';
      description = "The Clerk daemon package to run.";
    };

    settings = lib.mkOption {
      type = settingsFormat.type;
      default = defaultSettings;
      example = {
        server.bind_to_address = [
          "127.0.0.1:6601"
          "/run/user/1000/clerk/clerkd.sock"
        ];
        mpd.address = "/run/user/1000/mpd/socket";
        random = {
          tracks = 10;
          artist_tag = "albumartist";
        };
        cache.batch_size = 5000;
      };
      description = ''
        Settings written to `clerkd.toml`.

        This maps directly to Clerk's native TOML config:
        - `server.bind_to_address`
        - `mpd.address`
        - `random.tracks`
        - `random.artist_tag`
        - `cache.batch_size`
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    xdg.configFile."clerk/clerkd.toml".source =
      settingsFormat.generate "clerkd.toml" cfg.settings;

    systemd.user.services.clerkd = {
      Unit = {
        Description = "Clerk daemon";
        After = [ "network.target" ];
      };

      Service = {
        Type = "simple";
        ExecStart = "${cfg.package}/bin/clerkd";
        Restart = "on-failure";
      };

      Install = {
        WantedBy = [ "default.target" ];
      };
    };
  };
}
