package main

import (
	"context"
	"fmt"
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
)

const piBlaster = "/dev/pi-blaster"

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
		Listen string
		Pin    int
		Max    float64
		Min    float64
		Steps  uint32
	}{}

	flag.StringVar(&opts.Listen, "listen", ":8080", "The address on which internal server runs.")
	flag.IntVar(&opts.Pin, "pin", 18, "The number of the BCM2835 pin to use.")
	flag.Float64Var(&opts.Max, "max", 1, "The maximum acceptable PWM value; must be more than --min.")
	flag.Float64Var(&opts.Min, "min", 0, "The minimum acceptable PWM valuel must be less than --max.")
	flag.Uint32Var(&opts.Steps, "steps", 20, "The number of steps between --min and --max.")
	flag.Parse()

	if opts.Min >= opts.Max {
		stdlog.Fatalf("--min must be less than --max; got %f and %f, respectively", opts.Min, opts.Max)
		return
	}

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.WithPrefix(logger, "ts", log.DefaultTimestampUTC)
	logger = log.WithPrefix(logger, "caller", log.DefaultCaller)

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		requestsTotal,
	)

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
		router.Handle("/", newServor(opts.Pin, opts.Min, opts.Max, opts.Steps, logger))

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

	if err := g.Run(); err != nil {
		stdlog.Fatal(err)
	}
}

type servor struct {
	pin      int
	position float64
	min      float64
	max      float64
	step     float64

	mu     sync.Mutex
	logger log.Logger
}

func newServor(pin int, min, max float64, steps uint32, logger log.Logger) *servor {
	return &servor{
		pin:      pin,
		position: 0,
		max:      max,
		min:      min,
		step:     (max - min) / float64(steps),
		logger:   logger,
	}
}

func (s *servor) set() error {
	if s.position > s.max {
		s.position = s.max
	}
	if s.position < s.min {
		s.position = s.min
	}

	f, err := os.OpenFile(piBlaster, os.O_WRONLY|os.O_APPEND, 0644)
	defer f.Close()
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(f, "%d=%f\n", s.pin, s.position)
	return err
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
			s.position += s.step
		case "/api/right":
			s.mu.Lock()
			defer s.mu.Unlock()
			s.position -= s.step
		}
		if err := s.set(); err != nil {
			level.Error(s.logger).Log("err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
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
