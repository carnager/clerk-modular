{
  description = "Clerk MPD queue and rating tools";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];

      forAllSystems = f:
        nixpkgs.lib.genAttrs systems (system: f system);
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
          lib = pkgs.lib;
          version =
            if self ? shortRev then "unstable-${self.shortRev}"
            else "unstable";

          commonArgs = {
            inherit version;
            src = lib.cleanSource ./.;
            vendorHash = null;
            ldflags = [
              "-s"
              "-w"
            ];
          };

          clerkd = pkgs.buildGoModule (commonArgs // {
            pname = "clerkd";
            subPackages = [ "clerkd" ];
            meta.mainProgram = "clerkd";
          });

          clerk-rofi = pkgs.buildGoModule (commonArgs // {
            pname = "clerk-rofi";
            subPackages = [ "cmd/clerk-rofi" ];
            meta.mainProgram = "clerk-rofi";
          });

          clerk-musiclist = pkgs.buildGoModule (commonArgs // {
            pname = "clerk-musiclist";
            subPackages = [ "cmd/clerk-musiclist" ];
            meta.mainProgram = "clerk-musiclist";
          });

          clerk = pkgs.symlinkJoin {
            name = "clerk-${version}";
            paths = [
              clerkd
              clerk-rofi
              clerk-musiclist
            ];
          };
        in
        {
          inherit clerk clerkd clerk-rofi clerk-musiclist;
          default = clerk;
        });

      apps = forAllSystems (system:
        let
          packages = self.packages.${system};
        in
        {
          default = {
            type = "app";
            program = "${packages.clerkd}/bin/clerkd";
          };

          clerkd = {
            type = "app";
            program = "${packages.clerkd}/bin/clerkd";
          };

          clerk-rofi = {
            type = "app";
            program = "${packages.clerk-rofi}/bin/clerk-rofi";
          };

          clerk-musiclist = {
            type = "app";
            program = "${packages.clerk-musiclist}/bin/clerk-musiclist";
          };
        });

      devShells = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              gnumake
              nixfmt-rfc-style
            ];
          };
        });

      formatter = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        pkgs.nixfmt-rfc-style);

      homeManagerModules = {
        clerkd = import ./modules/home-manager/clerkd.nix { inherit self; };
        default = self.homeManagerModules.clerkd;
      };
    };
}
