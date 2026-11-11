INSERT INTO geo_states(place_id) VALUES($1) ON CONFLICT(place_id) DO UPDATE SET
state='unknown',candidate='',candidate_since=NULL,last_observed_at=NULL,last_location_id=''
