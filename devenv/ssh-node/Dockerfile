FROM ubuntu:26.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        openssh-server \
        python3 \
        python3-apt \
        sudo \
        ca-certificates \
        iproute2 \
        iputils-ping \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/run/sshd

#TODO: Make sure the operator also works when it has to use a password to become root!
RUN useradd -m -s /bin/bash ansible \
    && echo 'ansible:ansiblepass' | chpasswd \
    && echo 'ansible ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/ansible \
    && chmod 0440 /etc/sudoers.d/ansible \
    && mkdir -p /home/ansible/.ssh \
    && chmod 700 /home/ansible/.ssh \
    && chown ansible:ansible /home/ansible/.ssh

RUN echo 'root:rootpass' | chpasswd \
    && mkdir -p /root/.ssh \
    && chmod 700 /root/.ssh

RUN sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin yes/'              /etc/ssh/sshd_config \
    && sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config \
    && sed -i 's/^#\?PubkeyAuthentication.*/PubkeyAuthentication yes/'     /etc/ssh/sshd_config

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 22

CMD ["/entrypoint.sh"]
