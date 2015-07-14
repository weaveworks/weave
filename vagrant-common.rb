$go_version = "1.4.2"

$go_path = "/usr/local/go/bin"

def install_packages(vm, pkgs)
  vm.provision :shell, :inline => <<SCRIPT
apt-get update -qq
apt-get install -qq -y --force-yes --no-install-recommends #{pkgs.join(' ')}
SCRIPT
end

def install_build_deps(vm, pkgs)
  vm.provision :shell, :inline => <<SCRIPT
export DEBIAN_FRONTEND=noninteractive
apt-key adv \
  --keyserver hkp://keyserver.ubuntu.com:80 \
  --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9
echo 'deb https://get.docker.io/ubuntu docker main' \
  > /etc/apt/sources.list.d/docker.list
SCRIPT
  install_packages(vm, pkgs)
end

def install_go_toochain(vm)
  vm.provision :shell, :inline => <<SCRIPT
curl -s https://storage.googleapis.com/golang/go#{$go_version}.linux-amd64.tar.gz \
  | tar xz -C /usr/local
#{$go_path}/go clean -i net
#{$go_path}/go install -tags netgo std
SCRIPT
end

def tweak_user_env(vm)
  script = <<SCRIPT
echo 'export GOPATH="${HOME}"' \
  >> ~vagrant/.profile
echo 'export PATH="${HOME}/bin:#{$go_path}:${PATH}"' \
  >> ~vagrant/.profile
ln -sf ~vagrant/src/github.com/weaveworks/weave ~vagrant/
sudo chown -R vagrant:vagrant ~vagrant/src
SCRIPT
  vm.provision :shell, :inline => script, :privileged => false
end

def tweak_docker_daemon(vm)
  vm.provision :shell, :inline => <<SCRIPT
usermod -a -G docker vagrant
sed -i -e's%-H fd://%-H fd:// -H tcp://0.0.0.0:2375 -s overlay%' /lib/systemd/system/docker.service
systemctl daemon-reload
systemctl start docker
systemctl enable docker
SCRIPT
end

def cleanup(vm)
  vm.provision :shell, :inline => <<SCRIPT
export DEBIAN_FRONTEND=noninteractive
## Who the hell thinks official images have to have both of these?
/etc/init.d/chef-client stop
/etc/init.d/puppet stop
apt-get -qq remove puppet chef
apt-get -qq autoremove
killall -9 chef-client
SCRIPT
end
