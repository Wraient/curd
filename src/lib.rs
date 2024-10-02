pub mod aniskip;
use aniskip::SkipTime;

pub struct Anime {
    pub anilist_id: Option<i32>,
    pub mal_id: Option<i32>,
    pub allanime_id: Option<String>,
    pub watching_ep: Option<i32>,
    pub playing_time: Option<i32>,
    pub skip_times: Option<Vec<SkipTime>>,
}

impl Anime {
    // Constructor to create a new instance with fields initially set to None
    pub fn new() -> Self {
        Anime {
            anilist_id: None,
            mal_id: None,
            allanime_id: None,
            watching_ep: None,
            playing_time: None,
            skip_times: None,
        }
    }

    pub fn anilist_id(&mut self, id: i32){
        self.anilist_id = Some(id);
    }

    // Method to set the `mal_id`
    pub fn mal_id(&mut self, id: i32){
        self.mal_id = Some(id);
    }

    // Method to set the `allanime_id`
    pub fn allanime_id(&mut self, id: String){
        self.allanime_id = Some(id);
    }

    // Method to set the `watching_ep`
    pub fn watching_ep(&mut self, ep: i32){
        self.watching_ep = Some(ep);
    }

    // Method to set the `playing_time`
    pub fn playing_time(&mut self, time: i32){
        self.playing_time = Some(time);
    }

    // Method to set the `skip_times`
    pub fn skip_times(&mut self, times: Vec<SkipTime>){
        self.skip_times = Some(times);
    }
}

pub struct Mpv {
    pub speed: f32,
}