FROM alpine:3.6

RUN adduser -D cloudfront-invalidation-operator
USER cloudfront-invalidation-operator

ADD tmp/_output/bin/cloudfront-invalidation-operator /usr/local/bin/cloudfront-invalidation-operator
