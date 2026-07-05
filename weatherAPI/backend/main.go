package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type ApiKey struct {
	OpenWeatherApi string `json:"openWeatherApi"`
}

type WeatherData struct {
	Name string `json:"name"`
	Main struct {
		Kelvin float64 `json:"temp"`
	} `json:"main"`
}

func loadApiConfig(filename string) (ApiKey, error) {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("err loading the API key")
		return ApiKey{}, err
	}

	var c ApiKey
	err = json.Unmarshal(bytes, &c)
	if err != nil {
		fmt.Println("err reading the API key")
		return ApiKey{}, err
	}
	return c, nil
}

func hello(w http.ResponseWriter, R *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("hello from go!\n"))
}

func query(city string) (WeatherData, error) {
	apiConfig, err := loadApiConfig("./config/.apiConfig")
	if err != nil {
		return WeatherData{}, err
	}

	resp, err := http.Get("https://api.openweathermap.org/data/2.5/weather?APPID=" + apiConfig.OpenWeatherApi + "&q" + city)
	if err != nil {
		return WeatherData{}, err
	}

	defer resp.Body.Close()

	var d WeatherData
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return WeatherData{}, err
	}

	return d, nil
}

func main() {

	http.HandleFunc("/hello", hello)
	http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request) {
		city := strings.SplitN(r.URL.Path, "/", 3)[2]
		data, err := query(city)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(data)

	})

	http.ListenAndServe(":8080", nil)

}
