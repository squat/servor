FROM scratch
LABEL maintainer="squat <lserven@gmail.com>"
ARG GOARCH
COPY bin/$GOARCH/servor /
ENTRYPOINT ["/servor"]
