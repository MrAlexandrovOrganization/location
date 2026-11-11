INSERT INTO geo_places(id,name,latitude,longitude,radius_m,confirmation_seconds)
SELECT $1,$2,$3,$4,$5,$6 WHERE (SELECT count(*) FROM geo_places WHERE enabled)<100
RETURNING version
