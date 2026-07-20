FROM ubuntu:26.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        openssh-server \
        git \
        nginx \
        fcgiwrap \
        spawn-fcgi \
        cgit \
        apache2-utils \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/run/sshd

# Git user with static password (also used for HTTP basic auth).
RUN useradd -m -s /bin/bash git \
    && echo 'git:gitpass' | chpasswd \
    && mkdir -p /home/git/.ssh /srv/git \
    && chmod 700 /home/git/.ssh \
    && chown -R git:git /home/git /srv/git

# cgit runs as the git user via fcgiwrap; give it a writable cache dir.
RUN mkdir -p /var/cache/cgit && chown git:git /var/cache/cgit

# SSH config: password + pubkey auth, no root login.
RUN sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/'   /etc/ssh/sshd_config \
    && sed -i 's/^#\?PubkeyAuthentication.*/PubkeyAuthentication yes/'    /etc/ssh/sshd_config \
    && sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin no/'               /etc/ssh/sshd_config

# HTTP basic auth: same git/gitpass credentials.
RUN htpasswd -bc /etc/nginx/git.htpasswd git gitpass

COPY nginx.conf    /etc/nginx/sites-available/default
COPY cgitrc        /etc/cgitrc
COPY entrypoint.sh /entrypoint.sh
COPY .gitconfig    /root/.gitconfig
RUN chmod +x /entrypoint.sh

# Bake the test repositories into the image: the e2e image must serve them
# without any runtime mounts, so the repo creation the devenv entrypoint does
# from a /data volume happens here at build time instead.
COPY testdata        /data
COPY create-repos.sh /create-repos.sh
RUN chmod +x /create-repos.sh && /create-repos.sh && rm /create-repos.sh

EXPOSE 22 80

CMD ["/entrypoint.sh"]
