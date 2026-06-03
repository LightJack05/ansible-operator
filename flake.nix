{
  description = "Kubebuilder project dev shell";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    kubebuilderShell.url = "git+https://gitea.lightjack.de/LightJack05/nix-library?dir=shells/kubebuilder";
    generalLib.url = "git+https://gitea.lightjack.de/LightJack05/nix-library?dir=lib/general";
    podmanLib.url = "git+https://gitea.lightjack.de/LightJack05/nix-library?dir=lib/podman";
    kindLib.url = "git+https://gitea.lightjack.de/LightJack05/nix-library?dir=lib/kind";
    # --- Optional libs (uncomment input + merge lines below to enable) ---
    # qemuLib.url = "git+https://gitea.lightjack.de/LightJack05/nix-library?dir=lib/qemu";
  };

  outputs = { self, nixpkgs, kubebuilderShell, generalLib, podmanLib, kindLib, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};

          # --- Add project-specific packages here ---
          extraPackages = [
          ];

          # --- Add project-specific shell hook here (env vars, startup messages, etc.) ---
          extraShellHook = ''
          echo "=> Running shell hook for docker setup..."
          export K8S_NAMESPACE=ansible-operator-system
          export DEV_NETWORK=ansible-operator
          export DEV_SUBNET="172.30.0.0/16"
          export KIND_EXPERIMENTAL_DOCKER_NETWORK=$DEV_NETWORK
          # Unset to ensure it doesn't run podman, docker is a requirement here
          export KIND_EXPERIMENTAL_PROVIDER=""
          export KUBECONFIG="$HOME/.kube/config.d/ansible-dev-env"

          if ! docker network inspect "$DEV_NETWORK" >/dev/null 2>&1; then
              echo "===> Creating docker network '$DEV_NETWORK' ($DEV_SUBNET)"
              docker network create --subnet="$DEV_SUBNET" "$DEV_NETWORK"
          else
              echo "===> Docker network '$DEV_NETWORK' already exists (if you did not use it before, make sure the subnet matches)"
          fi
          '';

          # --- Optional lib packages (uncomment matching input above to enable) ---
          optionalPackages = []
          # ++ qemuLib.packages.${system}
          ;

          # --- Optional lib hooks (uncomment matching input above to enable) ---
          optionalHook = ""
          # + qemuLib.shellHook
          ;
        in
        {
          default = pkgs.mkShell {
            name = "kubebuilder-dev-shell";
            packages = kubebuilderShell.shellConfig.${system}.packages
              ++ generalLib.packages.${system}
              ++ podmanLib.packages.${system}
              ++ kindLib.packages.${system}
              ++ optionalPackages
              ++ extraPackages;
            shellHook = kubebuilderShell.shellConfig.${system}.shellHook
              + generalLib.shellHook
              + podmanLib.shellHook
              + optionalHook
              + extraShellHook;
          };
        }
      );
    };
}
