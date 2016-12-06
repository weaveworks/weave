FROM alpine
RUN apk --update add socat jq && rm -rf /var/cache/apk/*
ADD https://raw.githubusercontent.com/lehmannro/assert.sh/master/assert.sh /lib/assert.sh
COPY test-npc /bin/test-npc
ENTRYPOINT [ "/bin/test-npc" ]
