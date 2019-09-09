package main

import (
	"context"
	stdlog "log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	flag "github.com/spf13/pflag"
	"github.com/stianeikeland/go-rpio"
)

const (
	f       = 50
	rpioMin = 50000
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "The total the number of HTTP requests.",
		}, []string{"code", "handler", "method"},
	)
)

func main() {
	opts := struct {
		Frequency uint32
		Listen    string
		Max       uint32
		Min       uint32
		Pin       int
		Steps     uint32
	}{}

	flag.Uint32Var(&opts.Frequency, "frequency", f, "The frequency of the servo.")
	flag.StringVar(&opts.Listen, "listen", ":8080", "The address on which internal server runs.")
	flag.Uint32Var(&opts.Max, "max", 13, "The maximum duty cycle length as a percentage of the period.")
	flag.Uint32Var(&opts.Min, "min", 2, "The minimum duty cycle length as a percentage of the period.")
	flag.IntVar(&opts.Pin, "pin", 18, "The number of the BCM2835 number of the pin to use.")
	flag.Uint32Var(&opts.Steps, "steps", 100, "Break the period into this many steps.")
	flag.Parse()

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.WithPrefix(logger, "ts", log.DefaultTimestampUTC)
	logger = log.WithPrefix(logger, "caller", log.DefaultCaller)

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		requestsTotal,
	)

	if err := rpio.Open(); err != nil {
		level.Error(logger).Log("err", err)
		return
	}

	var g run.Group
	{
		// Signal chans must be buffered.
		sig := make(chan os.Signal, 1)
		g.Add(func() error {
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			<-sig
			return nil
		}, func(_ error) {
			level.Info(logger).Log("msg", "caught interrrupt")
			close(sig)
		})
	}
	{
		router := http.NewServeMux()
		router.Handle("/metrics", promhttp.InstrumentMetricHandler(reg, promhttp.HandlerFor(reg, promhttp.HandlerOpts{})))
		router.HandleFunc("/debug/pprof/", pprof.Index)
		router.Handle("/", newServor(opts.Pin, opts.Frequency, opts.Max, opts.Min, opts.Steps, logger))

		srv := &http.Server{Addr: opts.Listen, Handler: router}

		g.Add(func() error {
			level.Info(logger).Log("msg", "starting the HTTP server", "address", opts.Listen)
			return srv.ListenAndServe()
		}, func(err error) {
			if err == http.ErrServerClosed {
				level.Warn(logger).Log("msg", "internal server closed unexpectedly")
				return
			}
			level.Info(logger).Log("msg", "shutting down internal server")
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				stdlog.Fatal(err)
			}
		})
	}

	defer func() {
		if err := rpio.Close(); err != nil {
			level.Error(logger).Log("err", err)
		}
	}()

	if err := g.Run(); err != nil {
		stdlog.Fatal(err)
	}
}

type servor struct {
	rpio.Pin
	max      uint32
	min      uint32
	position uint32
	scale    uint32
	steps    uint32

	mu     sync.Mutex
	logger log.Logger
}

func newServor(pin int, frequency, max, min, steps uint32, logger log.Logger) *servor {
	p := rpio.Pin(pin)
	p.Pwm()
	scale := rpioMin / frequency / steps
	if scale == 0 {
		scale = 1
	}
	p.Freq(int(frequency * scale * steps))

	return &servor{
		Pin:      p,
		steps:    steps,
		logger:   logger,
		max:      max,
		min:      min,
		position: steps * min / 100,
		scale:    scale,
	}
}

func (s *servor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		switch r.URL.Path {
		case "/":
			fallthrough
		case "/index.html":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(html)); err != nil {
				level.Error(s.logger).Log("err", err)
			}
			return
		}
	case http.MethodPost:
		switch r.URL.Path {
		case "/api/left":
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.position != s.steps*s.max/100 {
				s.position++
			}
			s.DutyCycle(s.position*s.scale, s.steps*s.scale)
			w.WriteHeader(http.StatusOK)
			return
		case "/api/right":
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.position != s.steps*s.min/100 {
				s.position--
			}
			s.DutyCycle(s.position*s.scale, s.steps*s.scale)
			w.WriteHeader(http.StatusOK)
			return
		}
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

const html = `<!doctype html>
<html style="
    align-items: center;
    display: flex;
    height: 100%;
    justify-content: center;
    width: 100%;
">
<head>
  <meta charset="utf-8">
  <title>Servor</title>
  <meta name="description" content="">
  <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
    <div style="
	border: solid 5px;
        display: inline-block;
        font-family: sans-serif;
        font-size: 4em;
        font-weight: 500;
        line-height: 1;
        padding: .5em;
    ">
	<a href="https://github.com/squat/servor" style="
	    text-decoration: none;
	    color: #000;
	">servor</a>
	<div style="
    	    display: flex;
    	    justify-content: space-around;
    	">
	    <div id="left" style="
	        cursor: pointer;
	    ">←</div>
	    <div id="right" style="
	        cursor: pointer;
	    ">→</div>
	</div>
    </div>
    <script>
	servor = function(direction) {fetch('/api/'+direction, {method: 'POST'})};
	document.getElementById('left').onclick = function(e){
	    servor('left');
	    e.preventDefault();
	};
	document.getElementById('right').onclick = function(e){
	    servor('right');
	    e.preventDefault();
	};
        window.addEventListener('keydown', function (e) {
            switch (e.key) {
                case 'Left':
                case 'ArrowLeft':
		    servor('left');
                    break;
                case 'Right':
                case 'ArrowRight':
		    servor('right');
                    break;
                default:
                    return;
            }
            e.preventDefault();
        });
    </script>
</body>
</html>`
