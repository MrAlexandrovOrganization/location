UPDATE geo_places SET enabled=false,version=version+1 WHERE id=$1 AND enabled
