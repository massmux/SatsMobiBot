FROM golang:1.24.2-bookworm

RUN  	apt-get update && \
	apt-get -y install git wget

# installing satsmobi core
RUN 	cd /opt && \
	wget https://github.com/massmux/SatsMobiBot/archive/refs/tags/202504090700.tar.gz &&\
	tar xzvf 202504090700.tar.gz &&\
	mv SatsMobiBot-202504090700 SatsMobiBot

WORKDIR /opt/SatsMobiBot
COPY ./go.mod /opt/SatsMobiBot/go.mod

RUN	cd /opt/SatsMobiBot && go build .

CMD [ "./SatsMobiBot" ]
