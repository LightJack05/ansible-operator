FROM ubuntu:26.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        openssh-server \
        git \
        nginx \
        fcgiwrap \
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

# SSH config: password + pubkey auth, no root login.
RUN sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/'   /etc/ssh/sshd_config \
    && sed -i 's/^#\?PubkeyAuthentication.*/PubkeyAuthentication yes/'    /etc/ssh/sshd_config \
    && sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin no/'               /etc/ssh/sshd_config

# HTTP basic auth: same git/gitpass credentials.
RUN htpasswd -bc /etc/nginx/git.htpasswd git gitpass

COPY nginx.conf  /etc/nginx/sites-available/default
COPY entrypoint.sh /entrypoint.sh
COPY .gitconfig /root/.gitconfig
RUN chmod +x /entrypoint.sh

EXPOSE 22 80

CMD ["/entrypoint.sh"]
