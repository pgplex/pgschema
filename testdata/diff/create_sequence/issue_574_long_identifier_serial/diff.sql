CREATE TABLE IF NOT EXISTS measurement_data_collected_hourly (
    reading_identifier_value_number BIGSERIAL,
    label text,
    CONSTRAINT measurement_data_collected_hourly_pkey PRIMARY KEY (reading_identifier_value_number)
);
