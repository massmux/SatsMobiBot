FROM golang:1.24.2-bookworm

RUN  	apt-get update && \
	apt-get -y install git wget

COPY ./ /opt/SatsMobiBot/
WORKDIR /opt/SatsMobiBot

RUN	cd /opt/SatsMobiBot && go build .

EXPOSE	5454
EXPOSE	5588
EXPOSE	6060

CMD [ "./SatsMobiBot" ]
