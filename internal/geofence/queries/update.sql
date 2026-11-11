UPDATE geo_places SET name=$2,latitude=$3,longitude=$4,radius_m=$5,confirmation_seconds=$6,
version=version+1,created_at=now() WHERE id=$1 AND enabled RETURNING version
